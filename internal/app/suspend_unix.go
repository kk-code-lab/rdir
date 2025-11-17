//go:build !windows

package app

import (
	"syscall"

	"github.com/gdamore/tcell/v2"
)

func (app *Application) suspendToShell() {
	// Return terminal control to the shell before stopping the process.
	_ = app.screen.Suspend()
	_ = syscall.Kill(0, syscall.SIGTSTP)
}

func (app *Application) resumeAfterStop() bool {
	if err := app.screen.Resume(); err != nil {
		return false
	}
	app.screen.Sync()
	_ = app.screen.PostEvent(tcell.NewEventInterrupt("resume"))
	if w, h := app.screen.Size(); w > 0 && h > 0 {
		app.state.ScreenWidth = w
		app.state.ScreenHeight = h
	}
	return true
}
