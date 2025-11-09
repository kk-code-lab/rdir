package state

import (
	"os"
	"path/filepath"
	"testing"
)

// SIMPLE REGRESSION TEST for history bug
// Reproduces: go to dir1 (select pos 1) -> dir2 (select pos 1) -> back -> forward
// Expected: when going forward, should return to pos 1 in dir2

func TestHistorySimple_BackForward(t *testing.T) {
	// Create real temporary directories
	tmpDir, err := os.MkdirTemp("", "rdir-history-simple-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	if err := os.Mkdir(dir1, 0755); err != nil {
		t.Fatalf("Failed to create dir1: %v", err)
	}
	if err := os.Mkdir(dir2, 0755); err != nil {
		t.Fatalf("Failed to create dir2: %v", err)
	}

	// Create files
	for i := 0; i < 3; i++ {
		f := string(rune('a' + rune(i)))
		if err := os.WriteFile(filepath.Join(dir1, f+".txt"), []byte(f), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir2, f+".txt"), []byte(f), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	// Setup: state at dir1
	state := &AppState{
		CurrentPath:   dir1,
		History:       []string{dir1},
		HistoryIndex:  0,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, dir1); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// User navigates down to position 1 in dir1
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}
	if state.SelectedIndex != 1 {
		t.Fatalf("Expected position 1, got %d", state.SelectedIndex)
	}
	reducer.selectionHistory[state.CurrentPath] = state.SelectedIndex
	dir1Pos := state.SelectedIndex

	// Move to dir2 and add to history
	if err := reducer.changeDirectory(state, dir2); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	reducer.addToHistory(state, state.CurrentPath)

	// User navigates to position 1 in dir2
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}
	if state.SelectedIndex != 1 {
		t.Fatalf("Expected position 1 in dir2, got %d", state.SelectedIndex)
	}
	reducer.selectionHistory[state.CurrentPath] = state.SelectedIndex
	dir2Pos := state.SelectedIndex

	// GO BACK to dir1 using GoToHistoryAction
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "back"}); err != nil {
		t.Fatalf("Failed to go back in history: %v", err)
	}

	// Should be in dir1 with position restored
	if state.CurrentPath != dir1 {
		t.Errorf("After back, expected %s, got %s", dir1, state.CurrentPath)
	}
	if state.SelectedIndex != dir1Pos {
		t.Errorf("After back, expected position %d, got %d", dir1Pos, state.SelectedIndex)
	}

	// GO FORWARD to dir2
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "forward"}); err != nil {
		t.Fatalf("Failed to go forward in history: %v", err)
	}

	// THE BUG: Should be in dir2 with position restored to 1
	if state.CurrentPath != dir2 {
		t.Errorf("After forward, expected %s, got %s", dir2, state.CurrentPath)
	}
	if state.SelectedIndex != dir2Pos {
		t.Errorf("REGRESSION BUG: After forward, expected position %d in dir2, got %d", dir2Pos, state.SelectedIndex)
	}
}
