package pager

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

type keyKind int

const (
	keyUnknown keyKind = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
	keyEscape
	keyQuit
	keyToggleWrap
	keySpace
	keyCtrlC
	keyToggleHelp
	keyToggleInfo
	keyToggleFormat
	keyOpenEditor
	keyShiftUp
	keyShiftDown
	keyCopyVisible
	keyCopyAll
	keyStartSearch
	keyStartBinarySearch
	keySearchNext
	keySearchPrev
	keyToggleBinarySearchMode
	keyToggleBinarySearchLimit
	keyEnter
	keyBackspace
	keyRune
	keyJumpBackSmall
	keyJumpForwardSmall
	keyJumpBackLarge
	keyJumpForwardLarge
)

type keyEvent struct {
	kind keyKind
	ch   rune
	mod  int
}

func (p *PreviewPager) readKeyEvent() (keyEvent, error) {
	if p.reader == nil {
		return keyEvent{}, errors.New("no reader available")
	}

	b, err := p.reader.ReadByte()
	if err != nil {
		return keyEvent{}, err
	}

	switch b {
	case 0x1b:
		return p.parseEscapeSequence()
	case '?':
		return keyEvent{kind: keyToggleHelp, ch: rune(b)}, nil
	case 'k', 'K':
		return keyEvent{kind: keyUp, ch: rune(b)}, nil
	case 'j', 'J':
		return keyEvent{kind: keyDown, ch: rune(b)}, nil
	case 'h', 'H':
		return keyEvent{kind: keyToggleHelp, ch: rune(b)}, nil
	case 'q', 'Q':
		return keyEvent{kind: keyQuit, ch: rune(b)}, nil
	case 'x', 'X':
		return keyEvent{kind: keyQuit, ch: rune(b)}, nil
	case 'w', 'W':
		return keyEvent{kind: keyToggleWrap, ch: rune(b)}, nil
	case 'i', 'I':
		return keyEvent{kind: keyToggleInfo, ch: rune(b)}, nil
	case 'f', 'F':
		return keyEvent{kind: keyToggleFormat, ch: rune(b)}, nil
	case 'e', 'E':
		return keyEvent{kind: keyOpenEditor, ch: rune(b)}, nil
	case 'c':
		return keyEvent{kind: keyCopyVisible, ch: rune(b)}, nil
	case 'C':
		return keyEvent{kind: keyCopyAll, ch: rune(b)}, nil
	case '/':
		return keyEvent{kind: keyStartSearch, ch: rune(b)}, nil
	case ':':
		return keyEvent{kind: keyStartBinarySearch, ch: rune(b)}, nil
	case 'n':
		return keyEvent{kind: keySearchNext, ch: rune(b)}, nil
	case 'N':
		return keyEvent{kind: keySearchPrev, ch: rune(b)}, nil
	case 0x02: // Ctrl+B
		return keyEvent{kind: keyToggleBinarySearchMode}, nil
	case 0x0c: // Ctrl+L
		return keyEvent{kind: keyToggleBinarySearchLimit}, nil
	case ' ':
		return keyEvent{kind: keySpace, ch: rune(b)}, nil
	case 'b', 'B':
		return keyEvent{kind: keyPageUp, ch: rune(b)}, nil
	case 'g':
		return keyEvent{kind: keyHome, ch: rune(b)}, nil
	case 'G':
		return keyEvent{kind: keyEnd}, nil
	case '[':
		return keyEvent{kind: keyJumpBackSmall, ch: rune(b)}, nil
	case ']':
		return keyEvent{kind: keyJumpForwardSmall, ch: rune(b)}, nil
	case '{':
		return keyEvent{kind: keyJumpBackLarge, ch: rune(b)}, nil
	case '}':
		return keyEvent{kind: keyJumpForwardLarge, ch: rune(b)}, nil
	case '\r', '\n':
		return keyEvent{kind: keyEnter}, nil
	case 0x7f, 0x08:
		return keyEvent{kind: keyBackspace}, nil
	case 0x03:
		return keyEvent{kind: keyCtrlC}, nil
	default:
	}

	if b < utf8.RuneSelf {
		if b >= 0x20 {
			return keyEvent{kind: keyRune, ch: rune(b)}, nil
		}
		return keyEvent{kind: keyUnknown}, nil
	}

	buf := []byte{b}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		next, err := p.reader.ReadByte()
		if err != nil {
			break
		}
		buf = append(buf, next)
	}
	r, _ := utf8.DecodeRune(buf)
	if r != utf8.RuneError {
		return keyEvent{kind: keyRune, ch: r}, nil
	}
	return keyEvent{kind: keyUnknown}, nil
}

func (p *PreviewPager) parseEscapeSequence() (keyEvent, error) {
	if p.reader.Buffered() == 0 {
		return keyEvent{kind: keyEscape}, nil
	}
	next, err := p.reader.ReadByte()
	if err != nil {
		return keyEvent{kind: keyEscape}, nil
	}

	switch next {
	case '[':
		return p.parseCSI()
	case 'O':
		final, err := p.reader.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEscape}, nil
		}
		switch final {
		case 'H':
			return keyEvent{kind: keyHome}, nil
		case 'F':
			return keyEvent{kind: keyEnd}, nil
		default:
			return keyEvent{kind: keyUnknown}, nil
		}
	default:
		return keyEvent{kind: keyEscape}, nil
	}
}

func (p *PreviewPager) parseCSI() (keyEvent, error) {
	seq := []byte{}
	for {
		b, err := p.reader.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEscape}, nil
		}
		seq = append(seq, b)
		if isCSIFinalByte(b) {
			break
		}
		if len(seq) >= 16 {
			return keyEvent{kind: keyUnknown}, nil
		}
	}

	if len(seq) == 0 {
		return keyEvent{kind: keyUnknown}, nil
	}

	final := seq[len(seq)-1]
	base, modifier := parseCSIParameters(string(seq[:len(seq)-1]))

	switch final {
	case 'A':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftUp, mod: modifier}, nil
		}
		return keyEvent{kind: keyUp, mod: modifier}, nil
	case 'B':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftDown, mod: modifier}, nil
		}
		return keyEvent{kind: keyDown, mod: modifier}, nil
	case 'C':
		return keyEvent{kind: keyRight, mod: modifier}, nil
	case 'D':
		return keyEvent{kind: keyLeft, mod: modifier}, nil
	case 'H':
		return keyEvent{kind: keyHome, mod: modifier}, nil
	case 'F':
		return keyEvent{kind: keyEnd, mod: modifier}, nil
	case '~':
		switch base {
		case "5":
			return keyEvent{kind: keyPageUp, mod: modifier}, nil
		case "6":
			return keyEvent{kind: keyPageDown, mod: modifier}, nil
		case "1", "7":
			return keyEvent{kind: keyHome, mod: modifier}, nil
		case "4", "8":
			return keyEvent{kind: keyEnd, mod: modifier}, nil
		default:
			return keyEvent{kind: keyUnknown}, nil
		}
	}
	return keyEvent{kind: keyUnknown}, nil
}

func isCSIFinalByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || b == '~'
}

func parseCSIParameters(param string) (string, int) {
	if param == "" {
		return "", 1
	}

	parts := strings.Split(param, ";")
	if len(parts) == 0 {
		return "", 1
	}

	modifier := 1
	baseParts := parts
	if len(parts) > 1 {
		if val, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			modifier = val
			baseParts = parts[:len(parts)-1]
			if len(baseParts) == 0 {
				baseParts = []string{"1"}
			}
		}
	}
	base := strings.Join(baseParts, ";")
	return base, modifier
}

func hasShiftModifier(mod int) bool {
	switch mod {
	case 2, 4, 6, 8:
		return true
	default:
		return false
	}
}
