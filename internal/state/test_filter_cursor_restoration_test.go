package state

import (
	"testing"
)

// Test that cursor stays on the file selected in filter mode after exiting filter
func TestFilterCursorRestoration(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha.go", IsDir: false},
			{Name: "beta.go", IsDir: false},
			{Name: "gamma.go", IsDir: false},
			{Name: "delta.go", IsDir: false},
		},
		SelectedIndex: 0,
		ScrollOffset:  0,
		ScreenWidth:   80,
		ScreenHeight:  24,
	}

	// Start at index 0 (alpha.go)
	if state.SelectedIndex != 0 {
		t.Errorf("Initial selection setup failed: expected 0, got %d", state.SelectedIndex)
	}

	// Enter filter mode
	_, _ = reducer.Reduce(state, FilterStartAction{})

	if !state.FilterActive {
		t.Error("Filter should be active")
	}

	// Type filter query to match beta.go
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'b'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'e'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 't'})

	if state.FilterQuery != "bet" {
		t.Errorf("FilterQuery should be 'bet', got %q", state.FilterQuery)
	}

	if len(state.FilterMatches) == 0 {
		t.Error("Should have matches for pattern 'bet'")
	}

	// After typing, SelectedIndex should be on the first match (beta.go at index 1)
	if state.SelectedIndex != 1 {
		t.Errorf("After filter, should be on beta.go (index 1), got %d", state.SelectedIndex)
	}

	// Exit filter mode
	_, _ = reducer.Reduce(state, FilterClearAction{})

	// Verify state after exiting filter
	if state.FilterActive {
		t.Error("Filter should be inactive after clear")
	}

	if state.FilterQuery != "" {
		t.Errorf("FilterQuery should be empty, got %q", state.FilterQuery)
	}

	if len(state.FilterMatches) != 0 {
		t.Errorf("FilterMatches should be empty, got %d", len(state.FilterMatches))
	}

	// KEY TEST: Cursor should remain on the file selected in filter mode (beta.go at index 1)
	// NOT restore to the original position (alpha.go at index 0)
	if state.SelectedIndex != 1 {
		t.Errorf("SelectedIndex should stay on filtered selection (1), got %d", state.SelectedIndex)
	}
}

// Test that navigating in filter mode and then exiting keeps cursor on selected file
func TestFilterCursorRestoration_DifferentPositions(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.go", IsDir: false},
			{Name: "file2.go", IsDir: false},
			{Name: "file3.go", IsDir: false},
			{Name: "file4.go", IsDir: false},
			{Name: "file5.go", IsDir: false},
		},
		SelectedIndex: 0,
		ScreenWidth:   80,
		ScreenHeight:  24,
	}

	// Start at position 0 (file1.go)
	state.SelectedIndex = 0

	// Enter filter
	_, _ = reducer.Reduce(state, FilterStartAction{})

	if state.SelectedIndex != 0 {
		t.Errorf("After FilterStart, SelectedIndex should remain on current file (0), got %d", state.SelectedIndex)
	}

	// Type to filter - should match all files with 'f' (all of them)
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'f'})

	// Should be on first match (file1.go at index 0)
	firstMatchIndex := state.SelectedIndex
	if firstMatchIndex != 0 {
		t.Errorf("After filter, should be on first match (index 0), got %d", firstMatchIndex)
	}

	// Navigate down to select file2.go (index 1)
	_, _ = reducer.Reduce(state, NavigateDownAction{})

	if state.SelectedIndex != 1 {
		t.Errorf("After down, should be at index 1, got %d", state.SelectedIndex)
	}

	// Exit filter - cursor should stay on file2.go (index 1), not return to file1.go (index 0)
	_, _ = reducer.Reduce(state, FilterClearAction{})

	if state.SelectedIndex != 1 {
		t.Errorf("After FilterClear, should stay on selected file (index 1), got %d", state.SelectedIndex)
	}
}
