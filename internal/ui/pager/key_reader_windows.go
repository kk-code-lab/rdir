//go:build windows

package pager

import (
	"errors"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	evtKey       = 0x0001
	evtBufResize = 0x0004
)

// Minimal INPUT_RECORD definition for ReadConsoleInputW.
type inputRecord struct {
	EventType uint16
	_         uint16
	Event     [16]byte
}

type keyEventRecord struct {
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

var procReadConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

// startKeyReader implements a Windows-native key reader using ReadConsoleInputW.
func (p *PreviewPager) startKeyReader(done <-chan struct{}) (<-chan keyEvent, <-chan error, func()) {
	if p == nil || p.input == nil {
		errCh := make(chan error, 1)
		errCh <- errors.New("no pager input available")
		return nil, errCh, nil
	}

	handle := windows.Handle(p.input.Fd())
	events := make(chan keyEvent, 8)
	errCh := make(chan error, 1)

	// Disable VT input so console keeps emitting KEY_EVENT records (instead of VT
	// escape sequences) after tcell leaves the console in VT mode. Keep window
	// events so resize handling still works.
	var origMode uint32
	if modeErr := windows.GetConsoleMode(handle, &origMode); modeErr != nil {
		// Fall back to local reader (return nil channels) so pager remains usable.
		errCh <- modeErr
		return nil, errCh, nil
	}
	rawMode := origMode &^ windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	rawMode |= windows.ENABLE_WINDOW_INPUT
	if setErr := windows.SetConsoleMode(handle, rawMode); setErr != nil {
		// Also fall back to local reader if we cannot change the mode.
		errCh <- setErr
		return nil, errCh, nil
	}

	cancel, _ := windows.CreateEvent(nil, 1, 0, nil)
	stop := func() {
		_ = windows.SetConsoleMode(handle, origMode)
		if cancel != 0 {
			_ = windows.SetEvent(cancel)
			_ = windows.CloseHandle(cancel)
		}
	}

	go func() {
		defer close(events)
		defer close(errCh)
		defer stop()

		waitHandles := []windows.Handle{cancel, handle}
		var records [16]inputRecord

		for {
			wait, err := windows.WaitForMultipleObjects(waitHandles, false, windows.INFINITE)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}

			if wait == windows.WAIT_OBJECT_0 {
				return
			}

			var read uint32
			r1, _, e1 := procReadConsoleInput.Call(
				uintptr(handle),
				uintptr(unsafe.Pointer(&records[0])),
				uintptr(len(records)),
				uintptr(unsafe.Pointer(&read)),
			)
			if r1 == 0 {
				if e1 != nil {
					select {
					case errCh <- e1:
					default:
					}
				}
				return
			}

			for i := uint32(0); i < read; i++ {
				rec := records[i]
				switch rec.EventType {
				case evtKey:
					ev := (*keyEventRecord)(unsafe.Pointer(&rec.Event[0]))
					if ev == nil || ev.KeyDown == 0 {
						continue
					}
					if kev, ok := translateWindowsKey(ev); ok {
						select {
						case <-done:
							return
						case events <- kev:
						}
					}
				case evtBufResize:
					select {
					case <-done:
						return
					case events <- keyEvent{kind: keyUnknown}:
					}
				default:
				}
			}
		}
	}()

	go func() {
		<-done
		stop()
	}()

	return events, errCh, stop
}

func translateWindowsKey(ev *keyEventRecord) (keyEvent, bool) {
	if ev == nil {
		return keyEvent{}, false
	}
	vk := ev.VirtualKeyCode
	ch := rune(ev.UnicodeChar)

	switch vk {
	case windows.VK_UP:
		return keyEvent{kind: keyUp}, true
	case windows.VK_DOWN:
		return keyEvent{kind: keyDown}, true
	case windows.VK_LEFT:
		return keyEvent{kind: keyLeft}, true
	case windows.VK_RIGHT:
		return keyEvent{kind: keyRight}, true
	case windows.VK_PRIOR: // PageUp
		return keyEvent{kind: keyPageUp}, true
	case windows.VK_NEXT: // PageDown
		return keyEvent{kind: keyPageDown}, true
	case windows.VK_HOME:
		return keyEvent{kind: keyHome}, true
	case windows.VK_END:
		return keyEvent{kind: keyEnd}, true
	case windows.VK_ESCAPE:
		return keyEvent{kind: keyEscape}, true
	case windows.VK_RETURN:
		return keyEvent{kind: keyEnter}, true
	case windows.VK_BACK:
		return keyEvent{kind: keyBackspace}, true
	}

	if ch == 3 && (ev.ControlKeyState&(windows.LEFT_CTRL_PRESSED|windows.RIGHT_CTRL_PRESSED)) != 0 {
		return keyEvent{kind: keyCtrlC}, true
	}
	if ch != 0 {
		return runeToPagerKey(ch)
	}
	if ev.UnicodeChar == 0 {
		return keyEvent{}, false
	}
	if r := utf16.Decode([]uint16{ev.UnicodeChar}); len(r) == 1 {
		return runeToPagerKey(r[0])
	}
	return keyEvent{}, false
}

func runeToPagerKey(ch rune) (keyEvent, bool) {
	switch ch {
	case '?':
		return keyEvent{kind: keyToggleHelp, ch: ch}, true
	case 'k', 'K':
		return keyEvent{kind: keyUp, ch: ch}, true
	case 'j', 'J':
		return keyEvent{kind: keyDown, ch: ch}, true
	case 'h', 'H':
		return keyEvent{kind: keyToggleHelp, ch: ch}, true
	case 'q', 'Q':
		return keyEvent{kind: keyQuit, ch: ch}, true
	case 'x', 'X':
		return keyEvent{kind: keyQuit, ch: ch}, true
	case 'w', 'W':
		return keyEvent{kind: keyToggleWrap, ch: ch}, true
	case 'i', 'I':
		return keyEvent{kind: keyToggleInfo, ch: ch}, true
	case 'f', 'F':
		return keyEvent{kind: keyToggleFormat, ch: ch}, true
	case 'e', 'E':
		return keyEvent{kind: keyOpenEditor, ch: ch}, true
	case 'c':
		return keyEvent{kind: keyCopyVisible, ch: ch}, true
	case 'C':
		return keyEvent{kind: keyCopyAll, ch: ch}, true
	case '/':
		return keyEvent{kind: keyStartSearch, ch: ch}, true
	case 'n':
		return keyEvent{kind: keySearchNext, ch: ch}, true
	case 'N':
		return keyEvent{kind: keySearchPrev, ch: ch}, true
	case ' ':
		return keyEvent{kind: keySpace, ch: ch}, true
	case 'b', 'B':
		return keyEvent{kind: keyPageUp, ch: ch}, true
	case 'g':
		return keyEvent{kind: keyHome, ch: ch}, true
	case 'G':
		return keyEvent{kind: keyEnd, ch: ch}, true
	case '\b':
		return keyEvent{kind: keyBackspace}, true
	case '\r', '\n':
		return keyEvent{kind: keyEnter}, true
	case 0x03:
		return keyEvent{kind: keyCtrlC}, true
	}
	if ch >= 0x20 {
		return keyEvent{kind: keyRune, ch: ch}, true
	}
	return keyEvent{}, false
}
