package state

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ===== HIDDEN FILES TOGGLE TESTS =====
// Tests for the ToggleHiddenFilesAction and related cursor positioning behavior

func TestToggleHiddenFiles_Basic(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "visible1.txt", FullPath: filepath.Join("/test", "visible1.txt"), IsDir: false},
			{Name: ".hidden1", FullPath: filepath.Join("/test", ".hidden1"), IsDir: false},
			{Name: "visible2.txt", FullPath: filepath.Join("/test", "visible2.txt"), IsDir: false},
			{Name: ".hidden2", FullPath: filepath.Join("/test", ".hidden2"), IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()
	displayFiles := state.getDisplayFiles()

	// Should show only visible files
	if len(displayFiles) != 2 {
		t.Errorf("Expected 2 visible files, got %d", len(displayFiles))
	}

	// Toggle to show hidden
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	displayFiles = state.getDisplayFiles()
	if len(displayFiles) != 4 {
		t.Errorf("Expected 4 files after toggle, got %d", len(displayFiles))
	}
	if state.HideHiddenFiles != false {
		t.Errorf("Expected HideHiddenFiles to be false")
	}
}

func TestToggleHiddenFiles_CursorOnHidden(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "visible1.txt", FullPath: filepath.Join("/test", "visible1.txt"), IsDir: false},
			{Name: ".hidden1", FullPath: filepath.Join("/test", ".hidden1"), IsDir: false},
			{Name: "visible2.txt", FullPath: filepath.Join("/test", "visible2.txt"), IsDir: false},
			{Name: ".hidden2", FullPath: filepath.Join("/test", ".hidden2"), IsDir: false},
		},
		HideHiddenFiles: false,
		SelectedIndex:   1, // On .hidden1
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Hide hidden files - cursor should move to closest visible above or below
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected cursor to move to index 0 (visible1.txt), got %d", state.SelectedIndex)
	}
}

func TestToggleHiddenFiles_CursorBelowHidden(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: ".hidden1", FullPath: filepath.Join("/test", ".hidden1"), IsDir: false},
			{Name: ".hidden2", FullPath: filepath.Join("/test", ".hidden2"), IsDir: false},
			{Name: "visible1.txt", FullPath: filepath.Join("/test", "visible1.txt"), IsDir: false},
			{Name: "visible2.txt", FullPath: filepath.Join("/test", "visible2.txt"), IsDir: false},
		},
		HideHiddenFiles: false,
		SelectedIndex:   0, // On .hidden1
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Hide hidden files - cursor should move to closest visible below
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	if state.SelectedIndex != 2 {
		t.Errorf("Expected cursor to move to index 2 (visible1.txt), got %d", state.SelectedIndex)
	}
}

func TestToggleHiddenFiles_NoVisibleLeft(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: ".hidden1", FullPath: filepath.Join("/test", ".hidden1"), IsDir: false},
			{Name: ".hidden2", FullPath: filepath.Join("/test", ".hidden2"), IsDir: false},
		},
		HideHiddenFiles: false,
		SelectedIndex:   0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Hide hidden files - no visible files left
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	if state.SelectedIndex != -1 {
		t.Errorf("Expected cursor at -1 (no selection), got %d", state.SelectedIndex)
	}
}

func TestToggleHiddenFiles_ShowHiddenInEmpty(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: ".hidden1", IsDir: false},
			{Name: ".hidden2", IsDir: false},
		},
		HideHiddenFiles: true, // Initially hiding
		SelectedIndex:   -1,   // No selection
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Show hidden files - should select first
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected cursor at 0 (first hidden file), got %d", state.SelectedIndex)
	}
}

func TestToggleHiddenFiles_ScrolledListHideWhileShowing(t *testing.T) {
	// Simulate: long list with alternating visible/hidden files, scrolled down,
	// cursor at bottom on a visible file, then hide hidden files
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 100),
		HideHiddenFiles: false,
		SelectedIndex:   95, // Near end, on a visible file
		ScrollOffset:    80,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Populate with pattern: visible, hidden, visible, hidden, ...
	for i := 0; i < 100; i++ {
		isHidden := (i % 2) == 1
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	// Hide hidden files
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	// After hiding, display should only have visible files (50 files at indices 0, 2, 4, ..., 98)
	// We were at index 95 which is hidden (95 % 2 == 1), so cursor should move
	// Should find file above: index 94 is visible
	displayFiles := state.getDisplayFiles()
	if state.SelectedIndex < 0 {
		t.Errorf("Cursor disappeared: SelectedIndex=%d, displayFiles count=%d", state.SelectedIndex, len(displayFiles))
		return
	}

	// Check if selected file is visible
	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("Selected file not in display. SelectedIndex=%d, displayFiles count=%d", state.SelectedIndex, len(displayFiles))
	}
}

func TestToggleHiddenFiles_ScrolledListShowWhileHiding(t *testing.T) {
	// Simulate: long list of only visible files, scrolled down, cursor at bottom,
	// then show hidden files - should still have cursor
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 50),
		HideHiddenFiles: true, // Start with hidden files hidden
		SelectedIndex:   45,
		ScrollOffset:    40,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Populate with all visible files initially
	for i := 0; i < 50; i++ {
		state.Files[i] = FileEntry{Name: fmt.Sprintf("file%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	// Show hidden files
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	// Cursor should still exist and be at same position
	displayFiles := state.getDisplayFiles()
	if state.SelectedIndex < 0 {
		t.Errorf("Cursor disappeared: SelectedIndex=%d, displayFiles count=%d", state.SelectedIndex, len(displayFiles))
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("Selected file not in display. SelectedIndex=%d, displayFiles count=%d", state.SelectedIndex, len(displayFiles))
	}
}

func TestToggleHiddenFiles_ScrollOffsetBoundary(t *testing.T) {
	// Simulate the exact problem: long scrolled list, many hidden files
	// that will be hidden, causing ScrollOffset to exceed new display size
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 100),
		HideHiddenFiles: false,
		SelectedIndex:   95, // Visible file
		ScrollOffset:    80, // Scrolled down
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Populate: indices 0,2,4,...,98 are visible (50 total)
	// indices 1,3,5,...,99 are hidden (50 total)
	for i := 0; i < 100; i++ {
		isHidden := (i % 2) == 1
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	// Before toggle
	displayFilesB4 := state.getDisplayFiles()
	if len(displayFilesB4) != 100 { // Not filtering, not hiding yet
		t.Errorf("Before toggle: displayFiles should have 100, got %d", len(displayFilesB4))
	}
	if state.ScrollOffset != 80 {
		t.Errorf("Before toggle: ScrollOffset should be 80, got %d", state.ScrollOffset)
	}

	// Hide hidden files
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	// After toggle
	displayFilesAfter := state.getDisplayFiles()
	if len(displayFilesAfter) != 50 {
		t.Errorf("After toggle: displayFiles should have 50, got %d", len(displayFilesAfter))
	}

	// Critical check: cursor should still be visible and valid
	if state.SelectedIndex < 0 {
		t.Errorf("After toggle: cursor disappeared, SelectedIndex=%d", state.SelectedIndex)
		return
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("After toggle: getCurrentFile returned nil. SelectedIndex=%d, len(displayFiles)=%d, ScrollOffset=%d",
			state.SelectedIndex, len(displayFilesAfter), state.ScrollOffset)
		return
	}

	// Also check ScrollOffset is valid
	if state.ScrollOffset >= len(displayFilesAfter) {
		t.Errorf("After toggle: ScrollOffset=%d >= len(displayFiles)=%d", state.ScrollOffset, len(displayFilesAfter))
	}
}

func TestToggleHiddenFiles_DebugCursorInvisible(t *testing.T) {
	// Real-world scenario: many files, mostly hidden or visible,
	// scrolled down to show bottom, then toggle hide
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 200),
		HideHiddenFiles: false,
		SelectedIndex:   190,
		ScrollOffset:    170,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Pattern: every 3rd file is visible, rest hidden
	for i := 0; i < 200; i++ {
		isHidden := (i % 3) != 0
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE TOGGLE:")
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  Files[SelectedIndex].Name: %s", state.Files[state.SelectedIndex].Name)
	t.Logf("  ScrollOffset: %d", state.ScrollOffset)
	displayBefore := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayBefore))
	t.Logf("  getDisplaySelectedIndex(): %d", state.getDisplaySelectedIndex())

	// Toggle
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	t.Logf("AFTER TOGGLE:")
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("  Files[SelectedIndex].Name: %s", state.Files[state.SelectedIndex].Name)
	}
	t.Logf("  ScrollOffset: %d", state.ScrollOffset)
	displayAfter := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayAfter))
	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  getDisplaySelectedIndex(): %d", displayIdx)

	if displayIdx >= 0 && displayIdx < len(displayAfter) {
		t.Logf("  displayFiles[displayIdx].Name: %s", displayAfter[displayIdx].Name)
	} else {
		t.Logf("  displayIdx out of range!")
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: getCurrentFile returned nil")
		t.Logf("  Debugging: SelectedIndex=%d, len(displayFiles)=%d", state.SelectedIndex, len(displayAfter))
	}
}

func TestToggleHiddenFiles_CursorVisibilityOnScreen(t *testing.T) {
	// Check that after toggle, cursor is actually visible on screen
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 200),
		HideHiddenFiles: false,
		SelectedIndex:   190,
		ScrollOffset:    170,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Pattern: every 3rd file is visible
	for i := 0; i < 200; i++ {
		isHidden := (i % 3) != 0
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	// Toggle
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	// Check visibility
	visibleLines := state.visibleLines()
	displayIdx := state.getDisplaySelectedIndex()

	t.Logf("ScrollOffset: %d", state.ScrollOffset)
	t.Logf("displayIdx: %d", displayIdx)
	t.Logf("visibleLines: %d", visibleLines)
	t.Logf("Display range on screen: %d - %d", state.ScrollOffset, state.ScrollOffset+visibleLines-1)

	// Cursor must be in range [ScrollOffset, ScrollOffset+visibleLines)
	if displayIdx < state.ScrollOffset {
		t.Errorf("Cursor above screen: displayIdx=%d < ScrollOffset=%d", displayIdx, state.ScrollOffset)
	}
	if displayIdx >= state.ScrollOffset+visibleLines {
		t.Errorf("Cursor below screen: displayIdx=%d >= ScrollOffset+visibleLines=%d", displayIdx, state.ScrollOffset+visibleLines)
	}
}

func TestToggleHiddenFiles_WithActiveFilter(t *testing.T) {
	// Test toggling hidden files when filter is active
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 100),
		FilterActive:    true,
		FilterQuery:     "file",
		FilteredIndices: []int{},
		HideHiddenFiles: false,
		SelectedIndex:   50,
		ScrollOffset:    40,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Create files: every 2nd is hidden
	for i := 0; i < 100; i++ {
		isHidden := (i % 2) == 1
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	// Manually build FilteredIndices: all matching "file" (all of them)
	state.FilteredIndices = make([]int, 100)
	for i := 0; i < 100; i++ {
		state.FilteredIndices[i] = i
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE TOGGLE (with filter):")
	t.Logf("  FilterActive: %t", state.FilterActive)
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  getDisplaySelectedIndex(): %d", state.getDisplaySelectedIndex())

	// Toggle hidden files while filter is active
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed to toggle: %v", err)
	}

	t.Logf("AFTER TOGGLE (with filter):")
	t.Logf("  FilterActive: %t", state.FilterActive)
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  getDisplaySelectedIndex(): %d", displayIdx)
	displayFiles := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayFiles))

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: getCurrentFile returned nil")
	} else {
		t.Logf("  currentFile.Name: %s", currentFile.Name)
	}
}

func TestToggleHiddenFiles_RealScenario(t *testing.T) {
	// Real scenario from user:
	// 1. Short list with hidden files hidden (e.g., file0, file2, file4)
	// 2. Cursor on position 0 (file0)
	// 3. Show hidden files - now list is longer (file0, .hidden1, file2, .hidden3, file4...)
	// 4. Cursor still at position 0 (file0)
	// 5. Hide hidden files again
	// 6. PROBLEM: cursor disappears

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", FullPath: filepath.Join("/test", "file0"), IsDir: false},
			{Name: ".hidden1", FullPath: filepath.Join("/test", ".hidden1"), IsDir: false},
			{Name: "file2", FullPath: filepath.Join("/test", "file2"), IsDir: false},
			{Name: ".hidden3", FullPath: filepath.Join("/test", ".hidden3"), IsDir: false},
			{Name: "file4", FullPath: filepath.Join("/test", "file4"), IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   0, // file0
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("=== STEP 1: Initial state (hidden files hidden) ===")
	displayFiles := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayFiles))
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  getDisplaySelectedIndex(): %d", state.getDisplaySelectedIndex())
	currentFile := state.getCurrentFile()
	if currentFile != nil {
		t.Logf("  currentFile: %s", currentFile.Name)
	}

	// STEP 2: Show hidden files
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	t.Logf("=== STEP 2: After showing hidden files ===")
	displayFiles = state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayFiles))
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  getDisplaySelectedIndex(): %d", state.getDisplaySelectedIndex())
	currentFile = state.getCurrentFile()
	if currentFile != nil {
		t.Logf("  currentFile: %s", currentFile.Name)
	} else {
		t.Logf("  currentFile: NIL!")
	}

	// STEP 3: Hide hidden files again
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Step 3 failed: %v", err)
	}

	t.Logf("=== STEP 3: After hiding hidden files again ===")
	displayFiles = state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayFiles))
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  getDisplaySelectedIndex(): %d", displayIdx)
	currentFile = state.getCurrentFile()
	if currentFile != nil {
		t.Logf("  currentFile: %s", currentFile.Name)
	} else {
		t.Errorf("PROBLEM: currentFile is NIL!")
		t.Logf("  Files[SelectedIndex]: %s (hidden: %v)",
			state.Files[state.SelectedIndex].Name,
			state.Files[state.SelectedIndex].IsHidden())
	}

	// Final check
	if state.SelectedIndex < 0 {
		t.Errorf("FAIL: SelectedIndex is -1")
	}
	if state.getCurrentFile() == nil {
		t.Errorf("FAIL: cursor disappeared")
	}
}

func TestToggleHiddenFiles_CursorEndsOnHiddenFile(t *testing.T) {
	// Problem scenario:
	// 1. Hide hidden - cursor on file0 (visible)
	// 2. Show hidden - now full list visible, but SelectedIndex still points to file0
	// 3. But user navigated down to position 3 in display (which is .hidden3 in Files, index 3)
	// 4. Hide hidden again - cursor now on index 3 which IS hidden!

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   0, // file0
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Show hidden files
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}

	t.Logf("After show: SelectedIndex=%d, displayFiles=%d", state.SelectedIndex, len(state.getDisplayFiles()))

	// User navigates down to position 3 (which is .hidden3)
	state.SelectedIndex = 3
	t.Logf("User navigated to index 3 (.hidden3): displayFiles[3]=%s", state.getDisplayFiles()[3].Name)

	// Hide hidden files again
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	t.Logf("After hide: SelectedIndex=%d", state.SelectedIndex)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("  Files[SelectedIndex]: %s (hidden: %v)",
			state.Files[state.SelectedIndex].Name,
			state.Files[state.SelectedIndex].IsHidden())
	}

	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  getDisplaySelectedIndex(): %d", displayIdx)

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: cursor disappeared! SelectedIndex=%d should have moved away from hidden file", state.SelectedIndex)
	} else {
		t.Logf("  currentFile: %s", currentFile.Name)
	}
}

func TestToggleHiddenFiles_WithScrollOffset(t *testing.T) {
	// Test with significant scroll offset
	// Create many files, scroll down, then toggle
	state := &AppState{
		CurrentPath:     "/test",
		Files:           make([]FileEntry, 150),
		HideHiddenFiles: false,
		SelectedIndex:   140,
		ScrollOffset:    130,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	// Pattern: every 2nd file is hidden
	for i := 0; i < 150; i++ {
		isHidden := (i % 2) == 1
		name := "file"
		if isHidden {
			name = "." + name
		}
		state.Files[i] = FileEntry{Name: name + fmt.Sprintf("%d", i), IsDir: false}
	}

	reducer := NewStateReducer()

	t.Logf("BEFORE toggle:")
	t.Logf("  HideHiddenFiles: %t", state.HideHiddenFiles)
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  ScrollOffset: %d", state.ScrollOffset)
	displayBefore := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayBefore))

	// Toggle to hide
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Toggle failed: %v", err)
	}

	t.Logf("AFTER toggle:")
	t.Logf("  HideHiddenFiles: %t", state.HideHiddenFiles)
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  ScrollOffset: %d", state.ScrollOffset)
	displayAfter := state.getDisplayFiles()
	t.Logf("  len(displayFiles): %d", len(displayAfter))

	visibleLines := state.visibleLines()
	t.Logf("  visibleLines: %d", visibleLines)
	t.Logf("  ScrollOffset valid? %d < %d", state.ScrollOffset, len(displayAfter)-visibleLines+1)

	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("  getDisplaySelectedIndex(): %d", displayIdx)

	// Check if cursor is visible
	if displayIdx < 0 {
		t.Errorf("FAIL: displayIdx is -1 (cursor invisible)")
	}
	if displayIdx < state.ScrollOffset || displayIdx >= state.ScrollOffset+visibleLines {
		t.Errorf("FAIL: cursor not visible on screen (displayIdx=%d, ScrollOffset=%d, visibleLines=%d)",
			displayIdx, state.ScrollOffset, visibleLines)
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: getCurrentFile is nil")
	}
}

func TestToggleHiddenFiles_ExactUserScenarioWithNavigation(t *testing.T) {
	// Exact user scenario:
	// 1. Start: hidden hidden, cursor at 0
	// 2. Show hidden (.)
	// 3. Navigate down few times
	// 4. Hide hidden (.)
	// 5. Try to navigate - should work!

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		HideHiddenFiles: true,
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("=== INITIAL STATE ===")
	displayIdx := state.getDisplaySelectedIndex()
	t.Logf("HideHiddenFiles: %t, SelectedIndex: %d, displayIdx: %d",
		state.HideHiddenFiles, state.SelectedIndex, displayIdx)

	// STEP 1: Show hidden files
	t.Logf("=== SHOW HIDDEN FILES ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("HideHiddenFiles: %t, SelectedIndex: %d, displayIdx: %d, len(displayFiles): %d",
		state.HideHiddenFiles, state.SelectedIndex, displayIdx, len(state.getDisplayFiles()))

	// STEP 2: Navigate down
	t.Logf("=== NAVIGATE DOWN ===")
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("SelectedIndex: %d, displayIdx: %d, getCurrentFile: %v",
		state.SelectedIndex, displayIdx, state.getCurrentFile() != nil)

	// STEP 3: Navigate down again
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("SelectedIndex: %d, displayIdx: %d, getCurrentFile: %v",
		state.SelectedIndex, displayIdx, state.getCurrentFile() != nil)

	// STEP 4: Hide hidden files
	t.Logf("=== HIDE HIDDEN FILES ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	currentFile := state.getCurrentFile()
	t.Logf("HideHiddenFiles: %t, SelectedIndex: %d, displayIdx: %d",
		state.HideHiddenFiles, state.SelectedIndex, displayIdx)
	t.Logf("currentFile: %v", currentFile != nil)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("Files[SelectedIndex]: %s (hidden: %v)",
			state.Files[state.SelectedIndex].Name,
			state.Files[state.SelectedIndex].IsHidden())
	}

	// STEP 5: Try to navigate
	t.Logf("=== NAVIGATE DOWN (after hide) ===")
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	displayIdx = state.getDisplaySelectedIndex()
	currentFile = state.getCurrentFile()
	t.Logf("SelectedIndex: %d, displayIdx: %d, currentFile: %v",
		state.SelectedIndex, displayIdx, currentFile != nil)

	if currentFile == nil {
		t.Errorf("FAIL: cursor disappeared after hide!")
	}
}

func TestStartupWithHiddenFiles0(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: ".hidden0", IsDir: true}, // Files[0] is hidden
			{Name: "file1", IsDir: false},   // First visible file
			{Name: "file2", IsDir: false},
		},
		SelectedIndex:   0,
		HideHiddenFiles: true, // Hidden files are hidden at startup
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Simulate what happens at startup: changeDirectory calls resetViewport()
	state.resetViewport()

	displayIdx := state.getDisplaySelectedIndex()

	t.Logf("=== STARTUP STATE (after resetViewport) ===")
	t.Logf("HideHiddenFiles: %v", state.HideHiddenFiles)
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	t.Logf("displayIdx: %d", displayIdx)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("Files[SelectedIndex]: %s", state.Files[state.SelectedIndex].Name)
	}

	// After fix: resetViewport should have set SelectedIndex to first visible file
	if state.SelectedIndex != 1 { // Should be file1, not .hidden0
		t.Errorf("Expected SelectedIndex=1 (file1), got %d (%s)", state.SelectedIndex, state.Files[state.SelectedIndex].Name)
	}

	// Cursor should be visible
	if displayIdx < 0 {
		t.Errorf("Cursor should be visible, got displayIdx=%d", displayIdx)
	}

	// Toggle: Show hidden files
	t.Logf("=== AFTER TOGGLE (show hidden) ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed toggle: %v", err)
	}
	t.Logf("HideHiddenFiles: %v", state.HideHiddenFiles)
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	displayFiles := state.getDisplayFiles()
	t.Logf("displayFiles: %v", []string{displayFiles[0].Name, displayFiles[1].Name})

	if displayFiles[0].Name != ".hidden0" || displayFiles[1].Name != "file1" {
		t.Errorf("displayFiles order wrong")
	}

	// Toggle: Hide hidden files again
	t.Logf("=== AFTER TOGGLE (hide hidden) ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed toggle: %v", err)
	}
	t.Logf("HideHiddenFiles: %v", state.HideHiddenFiles)
	t.Logf("SelectedIndex: %d", state.SelectedIndex)
	displayIdx = state.getDisplaySelectedIndex()
	t.Logf("displayIdx: %d", displayIdx)
	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("getCurrentFile should not be nil")
	} else {
		t.Logf("currentFile: %s", currentFile.Name)
	}

	// Should still see first visible file (file1)
	if displayIdx < 0 {
		t.Errorf("After second toggle: cursor should still be visible, got displayIdx=%d", displayIdx)
	}
}

func TestSelectionHistoryRestoresVisibleWhenHiddenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	dirA := filepath.Join(tmpDir, "A")
	if err := os.Mkdir(dirA, 0o755); err != nil {
		t.Fatalf("failed to create dir A: %v", err)
	}

	hiddenFile := filepath.Join(dirA, ".hidden")
	if err := os.WriteFile(hiddenFile, []byte("hidden"), 0o644); err != nil {
		t.Fatalf("failed to create hidden file: %v", err)
	}
	ensureHidden(t, hiddenFile)
	if err := os.WriteFile(filepath.Join(dirA, "visible.txt"), []byte("visible"), 0o644); err != nil {
		t.Fatalf("failed to create visible file: %v", err)
	}

	state := &AppState{
		ScreenHeight:    24,
		ScreenWidth:     80,
		HideHiddenFiles: true,
	}
	reducer := NewStateReducer()

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to enter parent dir: %v", err)
	}

	// Enter directory A (hidden files currently hidden)
	if _, err := reducer.Reduce(state, EnterDirectoryAction{}); err != nil {
		t.Fatalf("enter directory action failed: %v", err)
	}
	if state.CurrentPath != dirA {
		t.Fatalf("expected to be in %s, got %s", dirA, state.CurrentPath)
	}

	// Show hidden files and navigate to the hidden entry
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("failed to show hidden files: %v", err)
	}
	if _, err := reducer.Reduce(state, NavigateUpAction{}); err != nil {
		t.Fatalf("failed to move to hidden file: %v", err)
	}
	current := state.getCurrentFile()
	if current == nil || current.Name != ".hidden" {
		t.Fatalf("expected to select hidden file, got %v", current)
	}

	// Go up to parent; this stores hidden selection in history
	if _, err := reducer.Reduce(state, GoUpAction{}); err != nil {
		t.Fatalf("go up action failed: %v", err)
	}

	// Hide hidden files before re-entering directory A
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("failed to hide hidden files: %v", err)
	}
	if !state.HideHiddenFiles {
		t.Fatalf("expected hidden files to be hidden")
	}

	// Re-enter directory A; cursor should land on first visible file
	if _, err := reducer.Reduce(state, EnterDirectoryAction{}); err != nil {
		t.Fatalf("re-enter directory action failed: %v", err)
	}

	current = state.getCurrentFile()
	if current == nil {
		t.Fatalf("expected a visible selection after re-entering")
	}
	if current.IsHidden() {
		t.Fatalf("expected visible selection, got hidden file %q", current.Name)
	}
	if current.Name != "visible.txt" {
		t.Fatalf("expected to select visible.txt, got %q", current.Name)
	}
}

func TestGoUpAfterHidingHiddenDirectoryKeepsSelectionVisible(t *testing.T) {
	tmpDir := t.TempDir()
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	if err := os.Mkdir(hiddenDir, 0o755); err != nil {
		t.Fatalf("failed to create hidden dir: %v", err)
	}
	ensureHidden(t, hiddenDir)
	if err := os.WriteFile(filepath.Join(hiddenDir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create file in hidden dir: %v", err)
	}
	visibleDir := filepath.Join(tmpDir, "visible")
	if err := os.Mkdir(visibleDir, 0o755); err != nil {
		t.Fatalf("failed to create visible dir: %v", err)
	}

	state := &AppState{
		ScreenHeight:    24,
		ScreenWidth:     80,
		HideHiddenFiles: false, // start with hidden entries visible
	}
	reducer := NewStateReducer()

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load temp dir: %v", err)
	}

	// Ensure the hidden directory is selected before entering
	hiddenIdx := -1
	for i, f := range state.Files {
		if f.Name == ".hidden" && f.IsDir {
			hiddenIdx = i
			break
		}
	}
	if hiddenIdx < 0 {
		t.Fatal("hidden directory not found in listing")
	}
	state.SelectedIndex = hiddenIdx

	// Enter the hidden directory
	if _, err := reducer.Reduce(state, EnterDirectoryAction{}); err != nil {
		t.Fatalf("enter hidden directory failed: %v", err)
	}
	if state.CurrentPath != hiddenDir {
		t.Fatalf("expected to be inside hidden dir, got %s", state.CurrentPath)
	}

	// Hide hidden entries while inside the hidden directory
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("toggle hidden files failed: %v", err)
	}
	if !state.HideHiddenFiles {
		t.Fatalf("expected hidden files to be hidden")
	}

	// Go up to parent; selection should remain on a visible entry
	if _, err := reducer.Reduce(state, GoUpAction{}); err != nil {
		t.Fatalf("go up action failed: %v", err)
	}
	if state.CurrentPath != tmpDir {
		t.Fatalf("expected to return to parent dir, got %s", state.CurrentPath)
	}

	displayIdx := state.getDisplaySelectedIndex()
	if displayIdx < 0 {
		t.Fatalf("expected visible selection after going up; got displayIdx=%d", displayIdx)
	}
	current := state.getCurrentFile()
	if current == nil {
		t.Fatal("expected a current file after going up")
	}
	if current.IsHidden() {
		t.Fatalf("expected visible selection, got hidden entry %q", current.Name)
	}
	if current.Name != "visible" {
		t.Fatalf("expected selection to land on visible dir, got %q", current.Name)
	}
}
