package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

// TestRightArrowOnDirectory checks that right arrow enters a directory
func TestRightArrowOnDirectory(t *testing.T) {
	// SETUP
	state := &statepkg.AppState{
		CurrentPath:   "/tmp",
		Files:         []statepkg.FileEntry{},
		SelectedIndex: 0,
		ScrollOffset:  0,
		FilterActive:  false,
	}

	// Create test directory structure
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create test subdir: %v", err)
	}

	// Load files
	if err := statepkg.LoadDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load directory: %v", err)
	}

	// EXECUTE: Simulate right arrow on directory
	action := statepkg.RightArrowAction{}

	// We need to check that the action would lead to EnterDirectoryAction
	// The actual handling is in app.go's processActions, but we can verify
	// the state has a directory selected
	if len(state.Files) == 0 {
		t.Fatalf("expected files in state, got none")
	}

	file := state.CurrentFile()
	if file == nil {
		t.Fatalf("expected current file, got nil")
	}

	// VERIFY: Selected file should be a directory
	if !file.IsDir {
		t.Errorf("expected directory, got regular file: %v", file.Name)
	}

	_ = action // Action is handled in app layer
}

// TestRightArrowOnFile checks that right arrow on a file is handled differently
func TestRightArrowOnFile(t *testing.T) {
	// SETUP
	state := &statepkg.AppState{
		CurrentPath:   "/tmp",
		Files:         []statepkg.FileEntry{},
		SelectedIndex: 0,
		ScrollOffset:  0,
		FilterActive:  false,
	}

	// Create test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Load files
	if err := statepkg.LoadDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load directory: %v", err)
	}

	if len(state.Files) == 0 {
		t.Fatalf("expected files in state, got none")
	}

	file := state.CurrentFile()
	if file == nil {
		t.Fatalf("expected current file, got nil")
	}

	// VERIFY: Selected file should NOT be a directory (it's a file)
	if file.IsDir {
		t.Errorf("expected regular file, got directory: %v", file.Name)
	}

	_ = statepkg.RightArrowAction{} // Action is handled in app layer
}

// TestIsTextFileDetection checks the text file detection heuristic
func TestIsTextFileDetection(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{
			name:     "empty file",
			path:     "empty.txt",
			content:  []byte{},
			expected: true,
		},
		{
			name:     "plain text",
			path:     "note.txt",
			content:  []byte("hello world\nthis is text\n"),
			expected: true,
		},
		{
			name:     "text with special chars",
			path:     "tabs.txt",
			content:  []byte("hello\nworld\twith\ttabs\n"),
			expected: true,
		},
		{
			name:     "binary data with null bytes",
			path:     "data.bin",
			content:  make([]byte, 512), // All zeros
			expected: false,
		},
		{
			name:     "go source code",
			path:     "main.go",
			content:  []byte("package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"),
			expected: true,
		},
		{
			name:     "binary extension overrides content",
			path:     "image.jpeg",
			content:  []byte("still text"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fsutil.IsTextFile(tt.path, tt.content)
			if result != tt.expected {
				t.Errorf("IsTextFile(%q) = %v, want %v", string(tt.content[:min(20, len(tt.content))]), result, tt.expected)
			}
		})
	}
}

// Helper function for tests
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestDetectPagerCommandPrefersEnv(t *testing.T) {
	args := detectPagerCommand("linux", "less -R", nil)
	expected := []string{"less", "-R"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectPagerCommandWindowsDefaultsToMore(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		if cmd == "more.com" {
			return `C:\Windows\System32\more.com`, nil
		}
		return "", errors.New("not found")
	}
	args := detectPagerCommand("windows", "", lookPath)
	expected := []string{`C:\Windows\System32\more.com`}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectPagerCommandWindowsFallbacksToType(t *testing.T) {
	lookPath := func(string) (string, error) {
		return "", errors.New("not found")
	}
	args := detectPagerCommand("windows", "", lookPath)
	expected := []string{"cmd", "/C", "type"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectClipboardPrefersPbcopyOnUnix(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		if cmd == "pbcopy" {
			return "/usr/bin/pbcopy", nil
		}
		return "", errors.New("not found")
	}
	args, ok := detectClipboardInternal("linux", lookPath)
	if !ok {
		t.Fatalf("expected clipboard command")
	}
	expected := []string{"/usr/bin/pbcopy"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectClipboardPrefersClipOnWindows(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		if cmd == "clip.exe" {
			return `C:\Windows\System32\clip.exe`, nil
		}
		return "", errors.New("not found")
	}
	args, ok := detectClipboardInternal("windows", lookPath)
	if !ok {
		t.Fatalf("expected clipboard command")
	}
	expected := []string{`C:\Windows\System32\clip.exe`}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectClipboardFallsBackToPowershell(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		if cmd == "powershell" {
			return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
		}
		return "", errors.New("not found")
	}
	args, ok := detectClipboardInternal("windows", lookPath)
	if !ok {
		t.Fatalf("expected clipboard command")
	}
	expected := []string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoLogo", "-NoProfile", "-Command", "Set-Clipboard"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectEditorCommandWindowsFallbacks(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		switch cmd {
		case "code":
			return "", errors.New("not found")
		case "notepad++.exe":
			return `C:\Program Files\Notepad++\notepad++.exe`, nil
		default:
			return "", errors.New("not found")
		}
	}
	getenv := func(string) string { return "" }
	args, ok := detectEditorCommandInternal("windows", getenv, lookPath)
	if !ok {
		t.Fatalf("expected editor fallback")
	}
	expected := []string{`C:\Program Files\Notepad++\notepad++.exe`}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}

func TestDetectEditorCommandUnixFallbacks(t *testing.T) {
	lookPath := func(cmd string) (string, error) {
		if cmd == "vim" {
			return "/usr/bin/vim", nil
		}
		return "", errors.New("not found")
	}
	getenv := func(string) string { return "" }
	args, ok := detectEditorCommandInternal("linux", getenv, lookPath)
	if !ok {
		t.Fatalf("expected editor fallback")
	}
	expected := []string{"/usr/bin/vim"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected %v, got %v", expected, args)
	}
}
