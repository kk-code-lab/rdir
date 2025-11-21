package app

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	inputui "github.com/kk-code-lab/rdir/internal/ui/input"
	renderui "github.com/kk-code-lab/rdir/internal/ui/render"
)

var debugLoggingEnabled = os.Getenv("RDIR_DEBUG_LOG") == "1"

// Application represents the running app.
type Application struct {
	screen         tcell.Screen
	state          *statepkg.AppState
	reducer        *statepkg.StateReducer
	renderer       *renderui.Renderer
	input          *inputui.InputHandler
	actionCh       chan statepkg.Action
	eventChan      chan tcell.Event
	eventStop      chan struct{}
	eventStopped   chan struct{}
	shouldQuit     bool
	currentPath    string
	clipboardCmd   []string
	clipboardAvail bool
	editorCmd      []string

	// Mouse state
	lastClickTime time.Time
	lastClickKey  string
}

// Close cleans up resources.
func (app *Application) Close() error {
	close(app.actionCh)
	app.stopEventPoller()
	if app.screen != nil {
		app.screen.Fini()
	}
	return nil
}

// GetCurrentPath returns the current directory to output on exit.
func (app *Application) GetCurrentPath() string {
	return app.currentPath
}

// GetCwd returns current working directory.
func GetCwd() (string, error) {
	return os.Getwd()
}

func (app *Application) startEventPoller() {
	if app.eventStop != nil {
		select {
		case <-app.eventStopped:
			app.eventStop = nil
			app.eventStopped = nil
		default:
			return
		}
	}
	app.eventChan = make(chan tcell.Event, 32)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	app.eventStop = stopCh
	app.eventStopped = doneCh

	go func(ch chan tcell.Event) {
		defer close(doneCh)
		app.screen.ChannelEvents(ch, stopCh)
	}(app.eventChan)
	app.logf("event poller started")
}

func (app *Application) stopEventPoller() {
	if app.eventStop == nil {
		return
	}
	stopCh := app.eventStop
	done := app.eventStopped
	app.eventChan = nil
	app.eventStop = nil
	app.eventStopped = nil

	close(stopCh)
	if done != nil {
		<-done
	}
	app.logf("event poller stopped")
}

// drainPendingEvents clears any queued tcell events. Useful after suspending
// the screen so old input (e.g., pager keystrokes) doesn't leak back in.
func (app *Application) drainPendingEvents() {
	app.logf("drain start")
	for app.screen != nil && app.screen.HasPendingEvent() {
		ev := app.screen.PollEvent()
		if ev == nil {
			break
		}
		app.logf("drain event: %s", formatTcellEvent(ev))
	}
	app.logf("drain end")
}

func (app *Application) logf(format string, args ...interface{}) {
	if !debugLoggingEnabled {
		return
	}
	log.Printf(format, args...)
}

func formatTcellEvent(ev tcell.Event) string {
	switch e := ev.(type) {
	case *tcell.EventKey:
		r := e.Rune()
		key := e.Key()
		name := tcell.KeyNames[key]
		if key == tcell.KeyRune {
			return fmt.Sprintf("key rune=%q mods=%v", r, e.Modifiers())
		}
		return fmt.Sprintf("key %v(%s) mods=%v", key, name, e.Modifiers())
	case *tcell.EventResize:
		w, h := e.Size()
		return fmt.Sprintf("resize %dx%d", w, h)
	default:
		return fmt.Sprintf("%T", ev)
	}
}
