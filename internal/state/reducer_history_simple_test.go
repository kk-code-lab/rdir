package state

import (
	"os"
	"path/filepath"
	"reflect"
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

// Scenario: A -> foo -> bar, then back (history) to foo, then navigate into bar.
// Navigating into the forward destination should reuse the forward entry instead
// of truncating it and appending a duplicate.
func TestHistory_BackThenUpReusesForward(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-history-upreuse-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	parent := tmpDir
	foo := filepath.Join(parent, "foo")
	bar := filepath.Join(foo, "bar")
	if err := os.MkdirAll(bar, 0755); err != nil {
		t.Fatalf("Failed to create bar: %v", err)
	}

	state := &AppState{
		CurrentPath:   parent,
		History:       []string{parent},
		HistoryIndex:  0,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, parent); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Enter foo
	if err := reducer.changeDirectory(state, foo); err != nil {
		t.Fatalf("Failed to change directory to foo: %v", err)
	}
	reducer.addToHistory(state, foo)

	// Enter bar
	if err := reducer.changeDirectory(state, bar); err != nil {
		t.Fatalf("Failed to change directory to bar: %v", err)
	}
	reducer.addToHistory(state, bar)

	// History now: [parent, foo, bar], index at last
	if got, want := state.History, []string{parent, foo, bar}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Unexpected history after setup: %v", got)
	}

	// Go back (history action) to foo; history unchanged, index moves back
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "back"}); err != nil {
		t.Fatalf("Failed to go back in history: %v", err)
	}
	if state.CurrentPath != foo || state.HistoryIndex != 1 {
		t.Fatalf("After back expected path %s idx 1, got %s idx %d", foo, state.CurrentPath, state.HistoryIndex)
	}

	// Now navigate into bar again. This should reuse forward entry (bar) instead
	// of truncating and appending duplicate.
	if err := reducer.changeDirectory(state, bar); err != nil {
		t.Fatalf("Failed to change directory to bar: %v", err)
	}
	reducer.addToHistory(state, bar)

	if state.CurrentPath != bar {
		t.Fatalf("After navigating to bar expected path %s, got %s", bar, state.CurrentPath)
	}

	if got, want := state.History, []string{parent, foo, bar}; !reflect.DeepEqual(got, want) {
		t.Fatalf("History should be unchanged, got %v", got)
	}
	if state.HistoryIndex != 2 {
		t.Fatalf("Expected HistoryIndex advanced to 2, got %d", state.HistoryIndex)
	}
}

// Scenario: enter foo, then go up immediately. History pointer should move back,
// keeping forward available so ']' can return to foo; no new entries added.
func TestHistory_UpMovesBackKeepsForward(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-history-upundo-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	parent := tmpDir
	foo := filepath.Join(parent, "foo")
	if err := os.Mkdir(foo, 0755); err != nil {
		t.Fatalf("Failed to create foo: %v", err)
	}

	state := &AppState{
		CurrentPath:   parent,
		History:       []string{parent},
		HistoryIndex:  0,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, parent); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Enter foo (records entry)
	if err := reducer.changeDirectory(state, foo); err != nil {
		t.Fatalf("Failed to change directory to foo: %v", err)
	}
	reducer.addToHistory(state, foo)

	if got, want := state.History, []string{parent, foo}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Unexpected history after entering foo: %v", got)
	}
	if state.HistoryIndex != 1 {
		t.Fatalf("Expected HistoryIndex 1 after entering foo, got %d", state.HistoryIndex)
	}

	// Go up immediately; should move history index back to parent without truncating.
	if _, err := reducer.Reduce(state, GoUpAction{}); err != nil {
		t.Fatalf("Failed to go up: %v", err)
	}

	if state.CurrentPath != parent {
		t.Fatalf("After GoUp expected path %s, got %s", parent, state.CurrentPath)
	}

	if got, want := state.History, []string{parent, foo}; !reflect.DeepEqual(got, want) {
		t.Fatalf("History should retain forward entry, got %v", got)
	}
	if state.HistoryIndex != 0 {
		t.Fatalf("Expected HistoryIndex 0 after moving back, got %d", state.HistoryIndex)
	}

	// Press forward should return to foo.
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "forward"}); err != nil {
		t.Fatalf("Forward action failed: %v", err)
	}
	if state.CurrentPath != foo || state.HistoryIndex != 1 {
		t.Fatalf("Forward should move to foo; path %s idx %d", state.CurrentPath, state.HistoryIndex)
	}
}

// Scenario from mid-history: A -> foo -> bar, back to foo, then GoUp (to A).
// GoUp should act like another history-back (pointer moves to A) and keep
// forward chain intact (two forwards return to bar).
func TestHistory_MidListGoUpActsAsBack(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-history-midback-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	parent := tmpDir
	foo := filepath.Join(parent, "foo")
	bar := filepath.Join(foo, "bar")
	if err := os.MkdirAll(bar, 0755); err != nil {
		t.Fatalf("Failed to create dirs: %v", err)
	}

	state := &AppState{
		CurrentPath:   parent,
		History:       []string{parent},
		HistoryIndex:  0,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, parent); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Enter foo then bar
	requireChange := func(p string) {
		if err := reducer.changeDirectory(state, p); err != nil {
			t.Fatalf("Failed to change directory to %s: %v", p, err)
		}
		reducer.addToHistory(state, p)
	}
	requireChange(foo)
	requireChange(bar)

	if got, want := state.History, []string{parent, foo, bar}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Unexpected history after setup: %v", got)
	}

	// Back to foo via history
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "back"}); err != nil {
		t.Fatalf("Back failed: %v", err)
	}
	if state.CurrentPath != foo || state.HistoryIndex != 1 {
		t.Fatalf("Expected foo idx 1, got %s idx %d", state.CurrentPath, state.HistoryIndex)
	}

	// GoUp should act like another back to parent
	if _, err := reducer.Reduce(state, GoUpAction{}); err != nil {
		t.Fatalf("GoUp failed: %v", err)
	}
	if state.CurrentPath != parent || state.HistoryIndex != 0 {
		t.Fatalf("Expected parent idx 0 after GoUp, got %s idx %d", state.CurrentPath, state.HistoryIndex)
	}

	// Two forwards should navigate through foo to bar
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "forward"}); err != nil {
		t.Fatalf("Forward1 failed: %v", err)
	}
	if state.CurrentPath != foo || state.HistoryIndex != 1 {
		t.Fatalf("Forward1 expected foo idx1, got %s idx %d", state.CurrentPath, state.HistoryIndex)
	}
	if _, err := reducer.Reduce(state, GoToHistoryAction{Direction: "forward"}); err != nil {
		t.Fatalf("Forward2 failed: %v", err)
	}
	if state.CurrentPath != bar || state.HistoryIndex != 2 {
		t.Fatalf("Forward2 expected bar idx2, got %s idx %d", state.CurrentPath, state.HistoryIndex)
	}
}
