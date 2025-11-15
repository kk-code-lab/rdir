package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestNormalizeClipboardPathWindows(t *testing.T) {
	input := `C:\Users\me/project/sub/file.txt`
	got := normalizeClipboardPath(input, "windows")
	want := `C:\Users\me\project\sub\file.txt`
	if got != want {
		t.Fatalf("normalizeClipboardPath(%q, windows) = %q, want %q", input, got, want)
	}
}

func TestNormalizeClipboardPathUnix(t *testing.T) {
	input := "/tmp/project/dir/../file.txt"
	got := normalizeClipboardPath(input, "linux")
	want := "/tmp/project/file.txt"
	if got != want {
		t.Fatalf("normalizeClipboardPath(%q, linux) = %q, want %q", input, got, want)
	}
}

func TestHandleClipboardSetsLastErrorOnFailure(t *testing.T) {
	app := newTestApplicationWithFile(t)
	app.clipboardAvail = true
	app.clipboardCmd = []string{"fake-clip", "--flag"}

	var recorded []string
	withFakeCommandBuilder(t, 7, &recorded, func() {
		app.handleClipboard()
	})

	if app.state.LastError == nil {
		t.Fatalf("expected clipboard failure to set LastError")
	}
	if got := app.state.LastError.Error(); !strings.Contains(got, "fake-clip") {
		t.Fatalf("expected error mentioning command, got %q", got)
	}
	if !app.state.LastYankTime.IsZero() {
		t.Fatalf("expected LastYankTime to remain zero on failure")
	}
	assertCommandRecorded(t, recorded, []string{"fake-clip", "--flag"})
}

func TestHandleClipboardUpdatesYankTimeOnSuccess(t *testing.T) {
	app := newTestApplicationWithFile(t)
	app.clipboardAvail = true
	app.clipboardCmd = []string{"fake-clip"}

	var recorded []string
	withFakeCommandBuilder(t, 0, &recorded, func() {
		app.handleClipboard()
	})

	if app.state.LastYankTime.IsZero() {
		t.Fatalf("expected LastYankTime to update on success")
	}
	if app.state.LastError != nil {
		t.Fatalf("expected LastError to remain nil on success, got %v", app.state.LastError)
	}
	assertCommandRecorded(t, recorded, []string{"fake-clip"})
}

func TestOpenFileInPagerFallbackPropagatesError(t *testing.T) {
	app := &Application{
		screen: newTestScreen(t),
	}
	args := []string{"fake-pager", "--raw"}

	var recorded []string
	var err error
	withFakeCommandBuilder(t, 3, &recorded, func() {
		err = app.openFileInPagerFallback(args)
	})

	if err == nil {
		t.Fatalf("expected error from pager fallback")
	}
	if got := err.Error(); !strings.Contains(got, "fake-pager") {
		t.Fatalf("expected pager error to include command name, got %q", got)
	}
	assertCommandRecorded(t, recorded, args)
}

func TestOpenFileInEditorFallbackPropagatesError(t *testing.T) {
	app := &Application{
		screen: newTestScreen(t),
	}
	args := []string{"fake-editor", "--wait"}

	var recorded []string
	var err error
	withFakeCommandBuilder(t, 5, &recorded, func() {
		err = app.openFileInEditorFallback(args)
	})

	if err == nil {
		t.Fatalf("expected error from editor fallback")
	}
	if got := err.Error(); !strings.Contains(got, "fake-editor") {
		t.Fatalf("expected editor error to include command name, got %q", got)
	}
	assertCommandRecorded(t, recorded, args)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Getenv("HELPER_PROCESS_EXIT"))
	if err != nil {
		code = 1
	}
	os.Exit(code)
}

func newTestApplicationWithFile(t *testing.T) *Application {
	t.Helper()
	dir := t.TempDir()
	file := "sample.txt"
	state := &statepkg.AppState{
		CurrentPath:   dir,
		Files:         []statepkg.FileEntry{{Name: file, FullPath: filepath.Join(dir, file)}},
		SelectedIndex: 0,
	}
	return &Application{
		state: state,
	}
}

func withFakeCommandBuilder(t *testing.T, exitCode int, recorded *[]string, fn func()) {
	t.Helper()
	orig := commandBuilder
	commandBuilder = func(name string, args ...string) *exec.Cmd {
		if recorded != nil {
			*recorded = append([]string{name}, args...)
		}
		return helperProcessCommand(exitCode, name, args...)
	}
	defer func() {
		commandBuilder = orig
	}()
	fn()
}

func helperProcessCommand(exitCode int, name string, args ...string) *exec.Cmd {
	cmdArgs := []string{"-test.run=TestHelperProcess", "--", name}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"HELPER_PROCESS_EXIT="+strconv.Itoa(exitCode),
	)
	return cmd
}

func newTestScreen(t *testing.T) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("failed to init screen: %v", err)
	}
	t.Cleanup(func() {
		screen.Fini()
	})
	return screen
}

func assertCommandRecorded(t *testing.T, recorded, want []string) {
	t.Helper()
	if len(recorded) != len(want) {
		t.Fatalf("expected command %v, got %v", want, recorded)
	}
	for i := range want {
		if recorded[i] != want[i] {
			t.Fatalf("expected command %v, got %v", want, recorded)
		}
	}
}
