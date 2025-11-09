package state

import (
	"testing"
)

// Test that Esc only works when filter is active
func TestEscOnlyWorksInFilterMode(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.go", IsDir: false},
			{Name: "file2.go", IsDir: false},
			{Name: "file3.go", IsDir: false},
		},
		SelectedIndex: 1,
		FilterActive:  false, // NOT in filter mode
		ScreenWidth:   80,
		ScreenHeight:  24,
	}

	initialIndex := state.SelectedIndex

	// Press Esc while NOT in filter mode
	_, _ = reducer.Reduce(state, FilterClearAction{})

	// Cursor should NOT change
	if state.SelectedIndex != initialIndex {
		t.Errorf("Esc outside filter mode should not change cursor: expected %d, got %d",
			initialIndex, state.SelectedIndex)
	}

	// Filter should still be inactive
	if state.FilterActive {
		t.Error("FilterActive should still be false")
	}
}

// Test that Esc works correctly when in filter mode
func TestEscWorksInFilterMode(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
			{Name: "file3.txt", IsDir: false},
		},
		SelectedIndex: 0,
		ScreenWidth:   80,
		ScreenHeight:  24,
	}

	// Enter filter mode
	_, _ = reducer.Reduce(state, FilterStartAction{})

	if !state.FilterActive {
		t.Error("Filter should be active")
	}

	// Type pattern that matches all files
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'f'})

	// After typing, should be on first match (file1.txt at index 0)
	if state.SelectedIndex != 0 {
		t.Errorf("After filter, should be on first match (0), got %d", state.SelectedIndex)
	}

	// Navigate down to file2.txt (index 1)
	_, _ = reducer.Reduce(state, NavigateDownAction{})

	if state.SelectedIndex != 1 {
		t.Errorf("After down in filter, should be at index 1, got %d", state.SelectedIndex)
	}

	// Navigate down again to file3.txt (index 2)
	_, _ = reducer.Reduce(state, NavigateDownAction{})

	if state.SelectedIndex != 2 {
		t.Errorf("After second down in filter, should be at index 2, got %d", state.SelectedIndex)
	}

	// Exit with Esc
	_, _ = reducer.Reduce(state, FilterClearAction{})

	// Filter should be cleared
	if state.FilterActive {
		t.Error("FilterActive should be false after Esc in filter mode")
	}

	if state.FilterQuery != "" {
		t.Errorf("FilterQuery should be empty, got %q", state.FilterQuery)
	}

	// Cursor should stay on file3.txt (index 2), not revert to file1.txt (index 0)
	if state.SelectedIndex != 2 {
		t.Errorf("Cursor should stay on selected file in filter (2), got %d", state.SelectedIndex)
	}
}

// Test that pressing Esc multiple times is safe
func TestEscMultipleTimes(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.go", IsDir: false},
			{Name: "file2.go", IsDir: false},
		},
		SelectedIndex: 1,
		ScreenWidth:   80,
		ScreenHeight:  24,
	}

	savedIndex := state.SelectedIndex

	// Press Esc multiple times (should be safe)
	for i := 0; i < 5; i++ {
		_, _ = reducer.Reduce(state, FilterClearAction{})
	}

	// Cursor should still be at saved position
	if state.SelectedIndex != savedIndex {
		t.Errorf("Esc multiple times should keep cursor at %d, got %d", savedIndex, state.SelectedIndex)
	}
}
