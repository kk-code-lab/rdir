package state

import (
	"fmt"
	"testing"
)

// ===== NAVIGATION TESTS =====

func TestNavigateDown(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
			{Name: "file3.txt", IsDir: false},
		},
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}

	if state.SelectedIndex != 1 {
		t.Errorf("Expected selected=1, got %d", state.SelectedIndex)
	}
}

func TestNavigateDownAtEnd(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
		},
		SelectedIndex: 1,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}

	if state.SelectedIndex != 1 {
		t.Errorf("Should stay at 1, got %d", state.SelectedIndex)
	}
}

func TestNavigateUp(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
		},
		SelectedIndex: 1,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, NavigateUpAction{}); err != nil {
		t.Fatalf("Failed to navigate up: %v", err)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected selected=0, got %d", state.SelectedIndex)
	}
}

func TestNavigateUpAtStart(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
		},
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, NavigateUpAction{}); err != nil {
		t.Fatalf("Failed to navigate up: %v", err)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Should stay at 0, got %d", state.SelectedIndex)
	}
}

func TestNavigationWithFilter(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "main.go"},
			{Name: "test.go"},
			{Name: "readme.txt"},
			{Name: "config.go"},
		},
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	// Start filter
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}

	// Type "go"
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'g'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'o'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	// Should have 3 matches: main.go, test.go, config.go
	if len(state.getDisplayFiles()) != 3 {
		t.Errorf("Expected 3 files matching 'go', got %d", len(state.getDisplayFiles()))
	}

	// Navigate in filtered list
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}
	if state.getDisplaySelectedIndex() != 1 {
		t.Errorf("Expected display index 1, got %d", state.getDisplaySelectedIndex())
	}

	// Clear filter
	if _, err := reducer.Reduce(state, FilterClearAction{}); err != nil {
		t.Fatalf("Failed to clear filter: %v", err)
	}

	// Should see all files again
	if len(state.getDisplayFiles()) != 4 {
		t.Errorf("Expected 4 files after clearing filter, got %d", len(state.getDisplayFiles()))
	}
}

func TestScrollPageDown(t *testing.T) {
	state := &AppState{
		CurrentPath:   "/test",
		Files:         make([]FileEntry, 100),
		SelectedIndex: 0,
		ScrollOffset:  0,
		ScreenHeight:  20, // 20 - 3 = 17 visible lines
	}

	// Populate files
	for i := 0; i < 100; i++ {
		state.Files[i].Name = string(rune('a' + rune(i%26)))
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ScrollPageDownAction{}); err != nil {
		t.Fatalf("Failed to scroll: %v", err)
	}

	// ScreenHeight 20 - 4 = 16 visible lines, so cursor should move 16 positions down
	if state.SelectedIndex != 16 {
		t.Errorf("Expected selected at 16, got %d", state.SelectedIndex)
	}
}

func TestScrollPageUp(t *testing.T) {
	state := &AppState{
		CurrentPath:   "/test",
		Files:         make([]FileEntry, 100),
		SelectedIndex: 25,
		ScrollOffset:  10,
		ScreenHeight:  20,
	}

	// Populate files
	for i := 0; i < 100; i++ {
		state.Files[i].Name = string(rune('a' + rune(i%26)))
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ScrollPageUpAction{}); err != nil {
		t.Fatalf("Failed to scroll: %v", err)
	}

	// ScreenHeight 20 - 4 = 16 visible lines, so cursor should move 16 positions up: 25 - 16 = 9
	if state.SelectedIndex != 9 {
		t.Errorf("Expected selected at 9, got %d", state.SelectedIndex)
	}
}

func TestScrollToStart(t *testing.T) {
	state := &AppState{
		CurrentPath:   "/test",
		Files:         make([]FileEntry, 10),
		SelectedIndex: 6,
		ScrollOffset:  4,
		ScreenHeight:  15,
	}
	for i := range state.Files {
		state.Files[i].Name = fmt.Sprintf("file%d", i)
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ScrollToStartAction{}); err != nil {
		t.Fatalf("Failed to scroll to start: %v", err)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected selection at start, got %d", state.SelectedIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("Expected scroll offset reset to 0, got %d", state.ScrollOffset)
	}
}

func TestScrollToEnd(t *testing.T) {
	state := &AppState{
		CurrentPath:   "/test",
		Files:         make([]FileEntry, 10),
		SelectedIndex: 0,
		ScrollOffset:  0,
		ScreenHeight:  12,
	}
	for i := range state.Files {
		state.Files[i].Name = fmt.Sprintf("file%d", i)
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ScrollToEndAction{}); err != nil {
		t.Fatalf("Failed to scroll to end: %v", err)
	}

	expected := len(state.Files) - 1
	if state.SelectedIndex != expected {
		t.Errorf("Expected selection at %d, got %d", expected, state.SelectedIndex)
	}
	displayFiles := state.getDisplayFiles()
	visibleLines := state.ScreenHeight - 4
	if len(displayFiles) > visibleLines && state.ScrollOffset != len(displayFiles)-visibleLines {
		t.Errorf("Expected scroll offset to align end of list, got %d", state.ScrollOffset)
	}
}

func TestNavigateDown_AfterAppStart(t *testing.T) {
	// Simulate: app starts, cursor at 0, user presses down
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: "file1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: "file3", IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE NavigateDown:")
	displayIdx := state.getDisplaySelectedIndex()
	displayFiles := state.getDisplayFiles()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)
	t.Logf("  len(displayFiles): %d", len(displayFiles))

	// Navigate down
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	t.Logf("AFTER NavigateDown:")
	displayIdx = state.getDisplaySelectedIndex()
	displayFiles = state.getDisplayFiles()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)
	t.Logf("  len(displayFiles): %d", len(displayFiles))
	t.Logf("  getCurrentFile: %s", state.getCurrentFile().Name)

	if state.SelectedIndex != 1 {
		t.Errorf("Expected SelectedIndex=1, got %d", state.SelectedIndex)
	}
}

func TestNavigateUp_FromPosition1(t *testing.T) {
	// Simulate: cursor at position 1, user presses up
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: "file1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: "file3", IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   1,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE NavigateUp:")
	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)

	// Navigate up
	if _, err := reducer.Reduce(state, NavigateUpAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	t.Logf("AFTER NavigateUp:")
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)

	if state.SelectedIndex != 0 {
		t.Errorf("Expected SelectedIndex=0, got %d", state.SelectedIndex)
	}
}

func TestNavigateDown_WithHiddenFiles(t *testing.T) {
	// Test navigation with hidden files visible
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		HideHiddenFiles: false, // Hidden files ARE visible
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE NavigateDown (with hidden visible):")
	displayIdx := state.getDisplaySelectedIndex()
	displayFiles := state.getDisplayFiles()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)
	t.Logf("  len(displayFiles): %d", len(displayFiles))
	t.Logf("  displayFiles: %v", []string{
		displayFiles[0].Name,
		displayFiles[1].Name,
		displayFiles[2].Name,
	})

	// Navigate down
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	t.Logf("AFTER NavigateDown:")
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  displayIdx: %d", displayIdx)
	t.Logf("  getCurrentFile: %s", state.getCurrentFile().Name)

	// Should be at .hidden1 (index 1 in Files, displayIdx 1 in displayFiles)
	if state.SelectedIndex != 1 {
		t.Errorf("Expected SelectedIndex=1, got %d", state.SelectedIndex)
	}
}

func TestAppStartNavigation(t *testing.T) {
	// Simulate app startup: default settings
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		HideHiddenFiles: true, // DEFAULT: hidden files are hidden
		SelectedIndex:   0,    // Start at first visible
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("=== APP START ===")
	displayIdx := state.getDisplaySelectedIndex()
	displayFiles := state.getDisplayFiles()
	t.Logf("HideHiddenFiles: %t", state.HideHiddenFiles)
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	t.Logf("displayIdx: %d", displayIdx)
	t.Logf("len(displayFiles): %d", len(displayFiles))
	if len(displayFiles) > 0 {
		t.Logf("Files: %v", []string{displayFiles[0].Name, displayFiles[1].Name})
	}

	// USER PRESSES DOWN
	t.Logf("=== NAVIGATE DOWN ===")
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	t.Logf("displayIdx: %d", displayIdx)
	t.Logf("getCurrentFile: %s", state.getCurrentFile().Name)

	if state.SelectedIndex != 2 { // Should go to file2 (next visible)
		t.Errorf("After DOWN: expected SelectedIndex=2 (file2), got %d", state.SelectedIndex)
	}

	// USER PRESSES UP
	t.Logf("=== NAVIGATE UP ===")
	if _, err := reducer.Reduce(state, NavigateUpAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	t.Logf("displayIdx: %d", displayIdx)
	t.Logf("getCurrentFile: %s", state.getCurrentFile().Name)

	if state.SelectedIndex != 0 { // Should go back to file0
		t.Errorf("After UP: expected SelectedIndex=0 (file0), got %d", state.SelectedIndex)
	}
}
