//go:build !windows

package app

import (
	"syscall"

	"github.com/gdamore/tcell/v2"
)

func (app *Application) suspendToShell() {
	// Return terminal control to the shell before stopping the process.
	_ = app.screen.Suspend()
	// Stop only this process; avoid signalling the entire process group
	// (which can include the wrapper shell function/process that launched
	// rdir, breaking job control like `fg`).
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)
}

func (app *Application) resumeAfterStop() bool {
	if err := app.screen.Resume(); err != nil {
		return false
	}
	// Re-enable mouse reporting after resume
	app.screen.EnableMouse()
	app.screen.Sync()
	_ = app.screen.PostEvent(tcell.NewEventInterrupt("resume"))
	if w, h := app.screen.Size(); w > 0 && h > 0 {
		app.state.ScreenWidth = w
		app.state.ScreenHeight = h
	}
	return true
}
