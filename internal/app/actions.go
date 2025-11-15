package app

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

var commandBuilder = exec.Command

func (app *Application) handleClipboard() bool {
	if app.clipboardAvail && len(app.clipboardCmd) > 0 {
		path := normalizeClipboardPath(app.state.CurrentFilePath(), runtime.GOOS)
		err := runExternalCommand(app.clipboardCmd, func(cmd *exec.Cmd) {
			cmd.Stdin = strings.NewReader(path)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}, "clipboard")
		if err != nil {
			app.state.LastError = err
			return true
		}
		app.state.LastYankTime = time.Now()
	}
	return true
}

func normalizeClipboardPath(inputPath string, goos string) string {
	if strings.EqualFold(goos, "windows") {
		cleaned := filepath.Clean(inputPath)
		return strings.ReplaceAll(cleaned, "/", `\`)
	}
	return path.Clean(filepath.ToSlash(inputPath))
}

func (app *Application) handleRightArrow() bool {
	file := app.state.CurrentFile()
	if file == nil {
		return true
	}

	if file.IsDir {
		if _, err := app.reducer.Reduce(app.state, statepkg.EnterDirectoryAction{}); err != nil {
			app.state.LastError = err
		}
		return true
	}

	if _, err := app.reducer.Reduce(app.state, statepkg.PreviewEnterFullScreenAction{}); err != nil {
		app.state.LastError = err
		return true
	}

	if app.state.PreviewData == nil || !app.state.PreviewFullScreen {
		return true
	}

	defer func() {
		if _, err := app.reducer.Reduce(app.state, statepkg.PreviewExitFullScreenAction{}); err != nil {
			app.state.LastError = err
		}
	}()

	if err := app.runPreviewPager(); err != nil {
		app.state.LastError = err
	}
	return true
}

func (app *Application) handleEditorOpen() bool {
	if !app.state.EditorAvailable || len(app.editorCmd) == 0 {
		return false
	}

	file := app.state.CurrentFile()
	if file == nil || file.IsDir {
		return false
	}

	filePath := filepath.Join(app.state.CurrentPath, file.Name)
	if err := app.openFileInEditor(filePath); err != nil {
		app.state.LastError = err
	}
	return true
}

func (app *Application) handleOpenPager() bool {
	file := app.state.CurrentFile()
	if file == nil || file.IsDir {
		return true
	}

	filePath := filepath.Join(app.state.CurrentPath, file.Name)
	if err := app.openFileInPager(filePath); err != nil {
		app.state.LastError = err
	}
	return true
}

func (app *Application) pagerArgs(filePath string) []string {
	base := detectPagerCommand(runtime.GOOS, os.Getenv("PAGER"), pagerLookPath)
	if len(base) == 0 {
		return nil
	}

	args := make([]string, len(base)+1)
	copy(args, base)
	args[len(base)] = filePath
	return args
}

func (app *Application) openFileInPager(filePath string) error {
	sample, err := fsutil.ReadTextSample(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	if !fsutil.IsTextFile(filePath, sample) {
		return nil
	}

	pagerArgs := app.pagerArgs(filePath)
	if len(pagerArgs) == 0 {
		return fmt.Errorf("no pager command available")
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return app.openFileInPagerFallback(pagerArgs)
	}
	defer func() {
		_ = tty.Close()
	}()

	if err := app.screen.Suspend(); err != nil {
		return fmt.Errorf("failed to suspend screen: %w", err)
	}

	runErr := runExternalCommand(pagerArgs, func(cmd *exec.Cmd) {
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
	}, "pager")

	if err := app.screen.Resume(); err != nil {
		return fmt.Errorf("failed to resume screen: %w", err)
	}
	app.screen.Sync()
	return runErr
}

func (app *Application) openFileInPagerFallback(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no pager command available")
	}
	if err := app.screen.Suspend(); err != nil {
		return fmt.Errorf("failed to suspend screen: %w", err)
	}
	defer func() {
		_ = app.screen.Resume()
	}()

	return runExternalCommand(args, func(cmd *exec.Cmd) {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}, "pager")
}

func (app *Application) openFileInEditor(filePath string) error {
	if len(app.editorCmd) == 0 {
		return fmt.Errorf("no editor configured")
	}

	editorArgs := app.editorArgsWithFile(filePath)
	useTTY := runtime.GOOS != "windows"
	var tty *os.File
	var err error

	if useTTY {
		tty, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return app.openFileInEditorFallback(editorArgs)
		}
		defer func() {
			_ = tty.Close()
		}()
	}

	if err := app.screen.Suspend(); err != nil {
		return fmt.Errorf("failed to suspend screen: %w", err)
	}

	runErr := runExternalCommand(editorArgs, func(cmd *exec.Cmd) {
		if useTTY {
			cmd.Stdin = tty
			cmd.Stdout = tty
			cmd.Stderr = tty
		} else {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
	}, "editor")

	if err := app.screen.Resume(); err != nil {
		return fmt.Errorf("failed to resume screen: %w", err)
	}
	app.screen.Sync()
	return runErr
}

func (app *Application) openFileInEditorFallback(args []string) error {
	if err := app.screen.Suspend(); err != nil {
		return fmt.Errorf("failed to suspend screen: %w", err)
	}
	defer func() {
		_ = app.screen.Resume()
		app.screen.Sync()
	}()

	return runExternalCommand(args, func(cmd *exec.Cmd) {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}, "editor")
}

func (app *Application) editorArgsWithFile(filePath string) []string {
	args := make([]string, len(app.editorCmd)+1)
	copy(args, app.editorCmd)
	args[len(app.editorCmd)] = filePath
	return args
}

func runExternalCommand(args []string, configure func(*exec.Cmd), label string) error {
	if len(args) == 0 {
		return fmt.Errorf("no %s command provided", label)
	}
	cmd := commandBuilder(args[0], args[1:]...)
	if configure != nil {
		configure(cmd)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s command %q failed: %w", label, args[0], err)
	}
	return nil
}
