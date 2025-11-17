//go:build windows

package app

// On Windows there is no SIGTSTP/SIGCONT; treat suspend as no-op.
func (app *Application) suspendToShell() {
}

func (app *Application) resumeAfterStop() bool {
	// Nothing to resume; keep running.
	return false
}
