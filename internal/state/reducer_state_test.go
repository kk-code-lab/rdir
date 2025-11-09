package state

import (
	"fmt"
	"testing"
)

// ===== STATE HELPER TESTS =====

func TestGetDisplayFiles_NoFilter(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "a.txt"},
			{Name: "b.txt"},
		},
		FilterActive: false,
	}

	display := state.getDisplayFiles()
	if len(display) != 2 {
		t.Errorf("Expected 2 files, got %d", len(display))
	}
}

func TestGetDisplayFiles_WithFilter(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "main.go", IsDir: false},
			{Name: "test.go", IsDir: false},
			{Name: "readme.txt", IsDir: false},
		},
		FilterActive:    true,
		FilteredIndices: []int{0, 1}, // main.go and test.go
	}

	display := state.getDisplayFiles()
	if len(display) != 2 {
		t.Errorf("Expected 2 filtered files, got %d", len(display))
	}

	if display[0].Name != "main.go" || display[1].Name != "test.go" {
		t.Error("Filtered results not in expected order")
	}
}

func TestSortFiles_DirectoriesFirst(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "z_file.txt", IsDir: false},
			{Name: "a_dir", IsDir: true},
			{Name: "b_file.txt", IsDir: false},
			{Name: "c_dir", IsDir: true},
		},
	}

	state.sortFiles()

	if !state.Files[0].IsDir {
		t.Error("First file should be a directory")
	}
	if !state.Files[1].IsDir {
		t.Error("Second file should be a directory")
	}
	if state.Files[2].IsDir {
		t.Error("Third file should not be a directory")
	}

	// Check alphabetical order within each group
	if state.Files[0].Name != "a_dir" {
		t.Errorf("Expected a_dir first, got %s", state.Files[0].Name)
	}
	if state.Files[2].Name != "b_file.txt" {
		t.Errorf("Expected b_file.txt, got %s", state.Files[2].Name)
	}
}

func TestUpdateScrollVisibility(t *testing.T) {
	state := &AppState{
		Files:         make([]FileEntry, 100),
		SelectedIndex: 50,
		ScrollOffset:  0,
		ScreenHeight:  20, // 20 - 3 = 17 visible lines
	}

	state.updateScrollVisibility()

	visibleLines := state.ScreenHeight - 4
	if state.ScrollOffset > 50 {
		t.Error("ScrollOffset should not be greater than selected index")
	}
	if state.SelectedIndex < state.ScrollOffset || state.SelectedIndex >= state.ScrollOffset+visibleLines {
		t.Error("Selected index should be visible after updateScrollVisibility")
	}
}

func TestRecomputeFilter_SmartCase(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "Main.go"},
			{Name: "IMPLEMENTATION.md"},
			{Name: "readme.TXT"},
		},
		FilterActive: true,
	}

	state.FilterQuery = "main"
	state.FilterCaseSensitive = false
	state.recomputeFilter()

	if len(state.FilteredIndices) != 2 {
		t.Errorf("Expected 2 matches for 'main', got %d", len(state.FilteredIndices))
	}

	if !containsIndex(state.FilteredIndices, 0) {
		t.Error("Lowercase query should include Main.go regardless of case")
	}
	if !containsIndex(state.FilteredIndices, 1) {
		t.Error("Lowercase query should include IMPLEMENTATION.md regardless of case")
	}

	state.FilterQuery = "MAIN"
	state.FilterCaseSensitive = true
	state.recomputeFilter()

	if len(state.FilteredIndices) != 1 {
		t.Errorf("Expected 1 match for 'MAIN' due to smart-case, got %d", len(state.FilteredIndices))
	}

	if !containsIndex(state.FilteredIndices, 1) {
		t.Error("Uppercase query should match IMPLEMENTATION.md exactly")
	}

	state.FilterQuery = "IMPLEMENT"
	state.FilterCaseSensitive = true
	state.recomputeFilter()

	if len(state.FilteredIndices) != 1 {
		t.Fatalf("Expected 1 match for 'IMPLEMENT', got %d", len(state.FilteredIndices))
	}

	if len(state.FilteredIndices) != 1 || state.FilteredIndices[0] != 1 {
		t.Error("Uppercase query should match IMPLEMENTATION.md exactly")
	}
}

func containsIndex(indices []int, target int) bool {
	for _, idx := range indices {
		if idx == target {
			return true
		}
	}
	return false
}

func TestResetViewport(t *testing.T) {
	state := &AppState{
		SelectedIndex:   10,
		ScrollOffset:    5,
		FilterActive:    true,
		FilterQuery:     "test",
		FilteredIndices: []int{0, 1, 2},
	}

	state.resetViewport()

	if state.SelectedIndex != 0 {
		t.Error("SelectedIndex should be 0")
	}
	if state.ScrollOffset != 0 {
		t.Error("ScrollOffset should be 0")
	}
	if state.FilterActive {
		t.Error("Filter should be cleared")
	}
	if state.FilterQuery != "" {
		t.Error("FilterQuery should be empty")
	}
}

func TestGetCurrentFile_NoSelection(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file.txt"},
		},
		SelectedIndex: 5, // Out of bounds
	}

	file := state.getCurrentFile()
	if file != nil {
		t.Error("Should return nil for out-of-bounds selection")
	}
}

func TestGetActualIndexFromDisplayIndex_NoFilterNoHide(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: "file1", IsDir: false},
			{Name: "file2", IsDir: false},
		},
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	// displayIdx should map directly
	if state.getActualIndexFromDisplayIndex(0) != 0 {
		t.Errorf("Expected 0, got %d", state.getActualIndexFromDisplayIndex(0))
	}
	if state.getActualIndexFromDisplayIndex(1) != 1 {
		t.Errorf("Expected 1, got %d", state.getActualIndexFromDisplayIndex(1))
	}
	if state.getActualIndexFromDisplayIndex(2) != 2 {
		t.Errorf("Expected 2, got %d", state.getActualIndexFromDisplayIndex(2))
	}
}

func TestGetActualIndexFromDisplayIndex_HideActive(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    false,
		HideHiddenFiles: true,
	}

	// Should skip hidden files
	// Display: [file0, file2, file4]
	// Index:   [0,     2,     4]
	if state.getActualIndexFromDisplayIndex(0) != 0 {
		t.Errorf("Expected 0, got %d", state.getActualIndexFromDisplayIndex(0))
	}
	if state.getActualIndexFromDisplayIndex(1) != 2 {
		t.Errorf("Expected 2, got %d", state.getActualIndexFromDisplayIndex(1))
	}
	if state.getActualIndexFromDisplayIndex(2) != 4 {
		t.Errorf("Expected 4, got %d", state.getActualIndexFromDisplayIndex(2))
	}
}

func TestGetActualIndexFromDisplayIndex_FilterActive(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    true,
		FilteredIndices: []int{0, 2, 4},
		HideHiddenFiles: false,
	}

	// displayIdx maps to FilteredIndices
	if state.getActualIndexFromDisplayIndex(0) != 0 {
		t.Errorf("Expected 0, got %d", state.getActualIndexFromDisplayIndex(0))
	}
	if state.getActualIndexFromDisplayIndex(1) != 2 {
		t.Errorf("Expected 2, got %d", state.getActualIndexFromDisplayIndex(1))
	}
	if state.getActualIndexFromDisplayIndex(2) != 4 {
		t.Errorf("Expected 4, got %d", state.getActualIndexFromDisplayIndex(2))
	}
}

func TestGetActualIndexFromDisplayIndex_FilterAndHideActive(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    true,
		FilteredIndices: []int{0, 1, 2, 3, 4}, // All match filter
		HideHiddenFiles: true,                 // But hide is active
	}

	// displayIdx should skip hidden in FilteredIndices
	// Display (visible only): [file0, file2, file4]
	// Indices in FilteredIndices: [0, 2, 4]
	if state.getActualIndexFromDisplayIndex(0) != 0 {
		t.Errorf("Expected 0, got %d", state.getActualIndexFromDisplayIndex(0))
	}
	if state.getActualIndexFromDisplayIndex(1) != 2 {
		t.Errorf("Expected 2, got %d", state.getActualIndexFromDisplayIndex(1))
	}
	if state.getActualIndexFromDisplayIndex(2) != 4 {
		t.Errorf("Expected 4, got %d", state.getActualIndexFromDisplayIndex(2))
	}
}

func TestGetCurrentFile_Valid(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file1.txt"},
			{Name: "file2.txt"},
		},
		SelectedIndex: 1,
	}

	file := state.getCurrentFile()
	if file == nil {
		t.Error("Should return a file")
		return
	}

	if file.Name != "file2.txt" {
		t.Errorf("Expected file2.txt, got %s", file.Name)
	}
}

// ===== SCROLL BEHAVIOR TESTS =====

func TestUpdateScrollVisibility_SelectionAboveScroll(t *testing.T) {
	state := &AppState{
		Files:           make([]FileEntry, 20),
		SelectedIndex:   1,
		ScrollOffset:    10,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: false,
	}
	// Create files
	for i := 0; i < 20; i++ {
		state.Files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
	}

	state.updateScrollVisibility()

	// Selected file (index 1) is above scroll offset (10), should scroll up to show it
	// displayIdx = 1, ScrollOffset starts at 10
	// Since 1 < 10, set ScrollOffset = 1
	if state.ScrollOffset != 1 {
		t.Errorf("Expected ScrollOffset=1, got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_SelectionBelowScroll(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0"},
			{Name: "file1"},
			{Name: "file2"},
			{Name: "file3"},
			{Name: "file4"},
			{Name: "file5"},
			{Name: "file6"},
		},
		SelectedIndex:   6,
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines (show items 0-5)
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.updateScrollVisibility()

	// Selected file is below visible range, should scroll down
	// displayIdx = 6, visibleLines = 6, scrollOffset + visibleLines = 6
	// So 6 >= 0 + 6, scroll: 6 - 6 + 1 = 1
	if state.ScrollOffset != 1 {
		t.Errorf("Expected ScrollOffset=1, got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_WithHideHiddenFiles(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: ".hidden1"},
			{Name: "file1"},
			{Name: ".hidden2"},
			{Name: "file2"},
			{Name: "file3"},
			{Name: "file4"},
		},
		SelectedIndex:   5, // Actual file index (file4)
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: true, // Hides .hidden1 and .hidden2
	}

	state.updateScrollVisibility()

	// Display files: [file1, file2, file3, file4] (indices 1,3,4,5 from Files)
	// displayIdx of SelectedIndex=5 is 3 (4th visible item)
	// With 6 visible lines, displayIdx=3 is within range [0,5]
	// So no scroll needed, should stay at 0
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0, got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_WithFilter(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "apple"},
			{Name: "banana"},
			{Name: "apricot"},
			{Name: "blueberry"},
			{Name: "avocado"},
		},
		SelectedIndex:   4,              // avocado
		FilteredIndices: []int{0, 2, 4}, // apple, apricot, avocado
		FilterActive:    true,
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		HideHiddenFiles: false,
	}

	state.updateScrollVisibility()

	// Display files: [apple, apricot, avocado] at indices [0, 2, 4]
	// SelectedIndex=4 maps to displayIdx=2
	// With 6 visible lines, displayIdx=2 is within [0,5]
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0, got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_FilterAndHideActive(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: ".hidden0"},
			{Name: "apple"},
			{Name: ".hidden1"},
			{Name: "apricot"},
			{Name: ".hidden2"},
			{Name: "avocado"},
		},
		SelectedIndex:   5,                       // avocado (visible)
		FilteredIndices: []int{0, 1, 2, 3, 4, 5}, // All match filter
		FilterActive:    true,
		HideHiddenFiles: true,
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
	}

	state.updateScrollVisibility()

	// Display files after filter+hide: [apple, apricot, avocado]
	// These are at Files indices [1, 3, 5]
	// SelectedIndex=5 maps to displayIdx=2
	// With 6 visible lines, displayIdx=2 is within [0,5]
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0, got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_NoSelection(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0"},
			{Name: "file1"},
		},
		SelectedIndex:   -1, // No selection
		ScrollOffset:    2,
		ScreenHeight:    10,
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.updateScrollVisibility()

	// With no selection, scroll should not be adjusted
	if state.ScrollOffset != 2 {
		t.Errorf("Expected ScrollOffset=2 (unchanged), got %d", state.ScrollOffset)
	}
}

func TestUpdateScrollVisibility_ScrollClamping(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0"},
			{Name: "file1"},
			{Name: "file2"},
		},
		SelectedIndex:   0,
		ScrollOffset:    100, // Way too high
		ScreenHeight:    10,  // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.updateScrollVisibility()

	// With only 3 files and 6 visible lines, maxOffset = 3 - 6 = -3 → 0
	// ScrollOffset should be clamped to 0
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0 (clamped), got %d", state.ScrollOffset)
	}
}

// ===== CENTER SCROLL ON SELECTION TESTS =====

func TestCenterScrollOnSelection_SmallList(t *testing.T) {
	// With fewer files than visible lines, should clamp to 0
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0"},
			{Name: "file1"},
			{Name: "file2"},
		},
		SelectedIndex:   2,
		ScrollOffset:    0,
		ScreenHeight:    20, // 20 - 4 = 16 visible lines
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.centerScrollOnSelection()

	// displayIdx = 2, visibleLines = 16
	// center = 2 - 16/2 = 2 - 8 = -6, clamped to 0
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0 (clamped), got %d", state.ScrollOffset)
	}
}

func TestCenterScrollOnSelection_LargeList(t *testing.T) {
	// Create 50 files
	files := make([]FileEntry, 50)
	for i := 0; i < 50; i++ {
		files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   25, // Middle file
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.centerScrollOnSelection()

	// displayIdx = 25, visibleLines = 6
	// center = 25 - 6/2 = 25 - 3 = 22
	if state.ScrollOffset != 22 {
		t.Errorf("Expected ScrollOffset=22, got %d", state.ScrollOffset)
	}
}

func TestCenterScrollOnSelection_WithHideHiddenFiles(t *testing.T) {
	files := make([]FileEntry, 20)
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			files[i] = FileEntry{Name: fmt.Sprintf(".hidden%d", i)}
		} else {
			files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
		}
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   19, // Last visible file (file19)
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: true,
		FilteredIndices: nil,
		FilterMatches:   nil,
	}

	displayIdx := state.getDisplaySelectedIndex()
	displayFiles := state.getDisplayFiles()
	visibleLines := state.ScreenHeight - 4
	maxOffset := len(displayFiles) - visibleLines

	t.Logf("displayIdx=%d, len(displayFiles)=%d, visibleLines=%d, maxOffset=%d",
		displayIdx, len(displayFiles), visibleLines, maxOffset)

	state.centerScrollOnSelection()

	// Files: [.hidden0, file1, .hidden2, file3, ..., .hidden18, file19]
	// Visible files (displayIdx counts visible files before SelectedIndex):
	// For SelectedIndex=19, count visible files from 0 to 18:
	// file1, file3, file5, ..., file17 = 9 visible files → displayIdx=9
	// len(displayFiles) = 10 (file1,3,5,7,9,11,13,15,17,19)
	// maxOffset = 10 - 6 = 4
	// center = 9 - 6/2 = 9 - 3 = 6, but clamped to maxOffset=4
	// So result should be 4
	if state.ScrollOffset != 4 {
		t.Errorf("Expected ScrollOffset=4 (clamped to maxOffset), got %d", state.ScrollOffset)
	}
}

func TestCenterScrollOnSelection_WithFilter(t *testing.T) {
	files := make([]FileEntry, 20)
	for i := 0; i < 20; i++ {
		files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   15,               // file15
		FilteredIndices: []int{5, 10, 15}, // Only 3 items match filter
		FilterActive:    true,
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		HideHiddenFiles: false,
	}

	state.centerScrollOnSelection()

	// Display: [file5, file10, file15] at files indices [5, 10, 15]
	// SelectedIndex=15 maps to displayIdx=2 (3rd item in filter)
	// center = 2 - 6/2 = 2 - 3 = -1, clamped to 0
	if state.ScrollOffset != 0 {
		t.Errorf("Expected ScrollOffset=0 (clamped), got %d", state.ScrollOffset)
	}
}

func TestCenterScrollOnSelection_NoSelection(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: "file0"},
			{Name: "file1"},
		},
		SelectedIndex:   -1, // No selection
		ScrollOffset:    5,
		ScreenHeight:    10,
		FilterActive:    false,
		HideHiddenFiles: false,
	}

	state.centerScrollOnSelection()

	// With no selection, scroll should not be adjusted
	if state.ScrollOffset != 5 {
		t.Errorf("Expected ScrollOffset=5 (unchanged), got %d", state.ScrollOffset)
	}
}

// ===== TOGGLE HIDDEN FILES WITH CENTER SCROLL TESTS =====

func TestToggleHiddenFiles_ManyHiddenAboveSelection(t *testing.T) {
	// Scenario: many hidden files above selected file
	// When hiding, they disappear and selection moves up in display
	// When showing, they appear and selection drops down in display
	files := make([]FileEntry, 30)
	// First 20 files are hidden (indices 0-19)
	// Files 20-29 are visible
	for i := 0; i < 30; i++ {
		if i < 20 {
			files[i] = FileEntry{Name: fmt.Sprintf(".hidden%d", i)}
		} else {
			files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
		}
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   25, // file25 (visible)
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: false,
		FilteredIndices: nil,
		FilterMatches:   nil,
	}

	// Scenario: toggle to hide hidden files
	// Display before: 30 files shown (displayIdx of 25 = 25)
	// Display after: only 10 visible files (file20-29), displayIdx of 25 = 5
	state.HideHiddenFiles = true
	state.invalidateDisplayFilesCache()
	displayIdx := state.getDisplaySelectedIndex()

	if displayIdx != 5 {
		t.Errorf("Expected displayIdx=5 after hide, got %d", displayIdx)
	}

	// Center: 5 - 6/2 = 5 - 3 = 2
	state.centerScrollOnSelection()

	if state.ScrollOffset != 2 {
		t.Errorf("Expected ScrollOffset=2 (centered), got %d", state.ScrollOffset)
	}
}

func TestToggleHiddenFiles_ShowingHiddenFilesAbove(t *testing.T) {
	// Scenario: reveal many hidden files above selection
	files := make([]FileEntry, 30)
	for i := 0; i < 30; i++ {
		if i < 20 {
			files[i] = FileEntry{Name: fmt.Sprintf(".hidden%d", i)}
		} else {
			files[i] = FileEntry{Name: fmt.Sprintf("file%d", i)}
		}
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   25, // file25 (visible)
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    false,
		HideHiddenFiles: true, // Currently hiding
		FilteredIndices: nil,
		FilterMatches:   nil,
	}

	// Display before hide: [file20-file29] (10 items), displayIdx of 25 = 5
	// Display after toggle: [.hidden0-29, file20-29] (30 items), displayIdx of 25 = 25
	state.HideHiddenFiles = false
	state.invalidateDisplayFilesCache()
	displayIdx := state.getDisplaySelectedIndex()

	if displayIdx != 25 {
		t.Errorf("Expected displayIdx=25 after show, got %d", displayIdx)
	}

	// Center: 25 - 6/2 = 25 - 3 = 22
	state.centerScrollOnSelection()

	if state.ScrollOffset != 22 {
		t.Errorf("Expected ScrollOffset=22 (centered), got %d", state.ScrollOffset)
	}
}

// ===== FILTER + TOGGLE HIDDEN EDGE CASES =====

func TestToggleHiddenFiles_WithFilter_AllResultsAreHidden(t *testing.T) {
	// Scenario: /sa filter matches only hidden files
	// Files that match /sa: .safari/, .scaffold/
	// Files that don't match: sanctuary/, sandbox/, kubescape, .omnisharp, etc
	files := []FileEntry{
		{Name: "sanctuary", IsDir: true},
		{Name: "sandbox", IsDir: true},
		{Name: ".safari", IsDir: true},    // Matches /sa filter, HIDDEN
		{Name: ".scaffold", IsDir: true},  // Matches /sa filter, HIDDEN
		{Name: ".kubescape", IsDir: true}, // Hidden but doesn't match /sa
		{Name: ".omnisharp", IsDir: true}, // Hidden
		{Name: "claude.json.backup", IsDir: false},
		{Name: ".iterm2_shell_integration.fish", IsDir: false},
		{Name: ".iterm2_shell_integration.zsh", IsDir: false},
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   3, // .scaffold (on a result that matches filter)
		ScrollOffset:    0,
		ScreenHeight:    10, // 10 - 4 = 6 visible lines
		FilterActive:    true,
		HideHiddenFiles: false,       // Currently showing hidden files
		FilteredIndices: []int{2, 3}, // .safari, .scaffold (both match /sa and are hidden)
		FilterMatches:   nil,
	}

	// BEFORE toggle: Filter active, HideHiddenFiles false
	// displayFiles = [.safari, .scaffold]
	// SelectedIndex=3 (.scaffold) maps to displayIdx=1
	displayIdx := state.getDisplaySelectedIndex()
	if displayIdx != 1 {
		t.Errorf("Before toggle - Expected displayIdx=1, got %d", displayIdx)
	}

	// NOW toggle to HIDE hidden files
	state.HideHiddenFiles = true
	state.invalidateDisplayFilesCache()

	// AFTER toggle: Filter active, HideHiddenFiles true
	// displayFiles should be empty (both filter results are hidden)
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 0 {
		t.Errorf("After toggle - Expected 0 display files, got %d", len(displayFiles))
	}

	// Note: When there are no visible files with filter+hide active,
	// getDisplaySelectedIndex() will still return >= 0 if SelectedIndex
	// points to a file in FilteredIndices (even if that file is hidden).
	// The actual cursor handling (setting to -1 or finding nearest visible)
	// is done by the ToggleHiddenFilesAction in the reducer.
	// This test just verifies the state functions work correctly.
}

func TestToggleHiddenFiles_WithFilter_AllResultsAreHidden_ThenShowAgain(t *testing.T) {
	// Scenario: Same as above, but then toggle BACK to show hidden files
	// User experience: filter active, toggle hide (no results), toggle show (results back)
	files := []FileEntry{
		{Name: "sanctuary", IsDir: true},
		{Name: "sandbox", IsDir: true},
		{Name: ".safari", IsDir: true},   // Matches /sa filter, HIDDEN
		{Name: ".scaffold", IsDir: true}, // Matches /sa filter, HIDDEN
		{Name: ".kubescape", IsDir: true},
		{Name: ".omnisharp", IsDir: true},
		{Name: "claude.json.backup", IsDir: false},
		{Name: ".iterm2_shell_integration.fish", IsDir: false},
		{Name: ".iterm2_shell_integration.zsh", IsDir: false},
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   3, // .scaffold
		ScrollOffset:    0,
		ScreenHeight:    10,
		FilterActive:    true,
		HideHiddenFiles: true,        // Currently hiding (after previous toggle)
		FilteredIndices: []int{2, 3}, // .safari, .scaffold
		FilterMatches:   nil,
	}

	// BEFORE toggle (hiding): No visible files
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 0 {
		t.Errorf("Before toggle show - Expected 0 display files, got %d", len(displayFiles))
	}

	// NOW toggle to SHOW hidden files
	state.HideHiddenFiles = false
	state.invalidateDisplayFilesCache()

	// AFTER toggle: Filter active, HideHiddenFiles false
	// displayFiles = [.safari, .scaffold] (both visible again)
	displayFiles = state.getDisplayFiles()
	if len(displayFiles) != 2 {
		t.Errorf("After toggle show - Expected 2 display files, got %d", len(displayFiles))
	}

	// Cursor should be restored to the one we had before
	// SelectedIndex=3 (.scaffold) should still be selected
	displayIdx := state.getDisplaySelectedIndex()
	if displayIdx != 1 {
		t.Errorf("After toggle show - Expected displayIdx=1, got %d", displayIdx)
	}

	// Center the selection
	state.centerScrollOnSelection()

	// With only 2 visible files, should clamp to sensible value
	if state.ScrollOffset < 0 {
		t.Errorf("After center - ScrollOffset should be >= 0, got %d", state.ScrollOffset)
	}
}

func TestToggleHiddenFiles_WithFilter_MixedResults(t *testing.T) {
	// Scenario: /sa filter matches both hidden and non-hidden files
	// Matches: sanctuary/, sandbox/ (visible), .safari/, .scaffold/ (hidden)
	files := []FileEntry{
		{Name: "sanctuary", IsDir: true}, // Matches /sa, VISIBLE
		{Name: "sandbox", IsDir: true},   // Matches /sa, VISIBLE
		{Name: ".safari", IsDir: true},   // Matches /sa, HIDDEN
		{Name: ".scaffold", IsDir: true}, // Matches /sa, HIDDEN
		{Name: ".kubescape", IsDir: true},
		{Name: ".omnisharp", IsDir: true},
		{Name: "claude.json.backup", IsDir: false},
	}

	state := &AppState{
		Files:           files,
		SelectedIndex:   2, // .safari (hidden result)
		ScrollOffset:    0,
		ScreenHeight:    10,
		FilterActive:    true,
		HideHiddenFiles: false,             // Showing all filter results
		FilteredIndices: []int{0, 1, 2, 3}, // sanctuary, sandbox, .safari, .scaffold
		FilterMatches:   nil,
	}

	// BEFORE toggle: All 4 results visible
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 4 {
		t.Errorf("Before toggle - Expected 4 display files, got %d", len(displayFiles))
	}

	// Cursor on .safari (hidden result)
	displayIdx := state.getDisplaySelectedIndex()
	if displayIdx != 2 {
		t.Errorf("Before toggle - Expected displayIdx=2, got %d", displayIdx)
	}

	// NOW toggle to HIDE hidden files
	state.HideHiddenFiles = true
	state.invalidateDisplayFilesCache()

	// AFTER toggle: Only 2 visible results (sanctuary, sandbox)
	displayFiles = state.getDisplayFiles()
	if len(displayFiles) != 2 {
		t.Errorf("After toggle hide - Expected 2 display files, got %d", len(displayFiles))
	}

	// Cursor should shift to nearest visible file (sanctuary at index 0)
	// This is handled by ToggleHiddenFilesAction logic in reducer
	// If SelectedIndex=2 (.safari) becomes hidden, it should pick nearest visible
	if state.SelectedIndex != 2 {
		// The reducer action would handle moving to nearest visible
		// For this unit test, we're just testing the state after toggle
		t.Logf("After hide - SelectedIndex: %d (would be adjusted by reducer logic)", state.SelectedIndex)
	}
}
