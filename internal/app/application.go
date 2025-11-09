package app

import (
	"os"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	inputui "github.com/kk-code-lab/rdir/internal/ui/input"
	renderui "github.com/kk-code-lab/rdir/internal/ui/render"
)

// Application represents the running app.
type Application struct {
	screen         tcell.Screen
	state          *statepkg.AppState
	reducer        *statepkg.StateReducer
	renderer       *renderui.Renderer
	input          *inputui.InputHandler
	actionCh       chan statepkg.Action
	shouldQuit     bool
	currentPath    string
	clipboardCmd   []string
	clipboardAvail bool
	editorCmd      []string
}

// Close cleans up resources.
func (app *Application) Close() error {
	close(app.actionCh)
	app.screen.Fini()
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
