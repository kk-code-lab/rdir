package state

import (
	"testing"
)

// ===== FILTER + HIDDEN FILES COMBINATION TESTS =====
// These test interactions between FilterActive and HideHiddenFiles

func TestFilterPlusHidden_ToggleShowWhileFiltering(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    true,
		FilterQuery:     "file",
		FilteredIndices: []int{0, 2, 4},
		HideHiddenFiles: true,
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	if state.getCurrentFile() == nil {
		t.Errorf("FAIL: currentFile is nil after toggle show")
	} else if state.getCurrentFile().Name != "file0" {
		t.Errorf("Expected file0, got %s", state.getCurrentFile().Name)
	}
}

func TestFilterPlusHidden_ToggleHideWhileFilteringWithHiddenSelected(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    true,
		FilterQuery:     "file",
		FilteredIndices: []int{0, 1, 2, 3, 4},
		HideHiddenFiles: false,
		SelectedIndex:   1, // .hidden1
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after toggle hide")
	} else if currentFile.IsHidden() {
		t.Errorf("FAIL: cursor should not be on hidden file after toggle")
	}
}

func TestFilterPlusHidden_NavigateInBothActive(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file0", IsDir: false},
			{Name: ".hidden1", IsDir: false},
			{Name: "file2", IsDir: false},
			{Name: ".hidden3", IsDir: false},
			{Name: "file4", IsDir: false},
		},
		FilterActive:    true,
		FilterQuery:     "file",
		FilteredIndices: []int{0, 1, 2, 3, 4},
		HideHiddenFiles: true,
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after navigate")
	} else if currentFile.Name != "file2" {
		t.Errorf("Expected file2, got %s", currentFile.Name)
	}
}

func TestFilterPlusHidden_ComplexToggleSequence(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple", IsDir: false},
			{Name: ".hidden_a", IsDir: false},
			{Name: "banana", IsDir: false},
			{Name: ".hidden_b", IsDir: false},
			{Name: "cherry", IsDir: false},
		},
		FilterActive:    true,
		FilterQuery:     "a",
		FilteredIndices: []int{0, 1, 2, 3, 4},
		HideHiddenFiles: false,
		SelectedIndex:   0,
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Navigate down
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Toggle hide
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after toggle")
	} else if currentFile.IsHidden() {
		t.Errorf("FAIL: cursor should not be hidden")
	}

	// Navigate again
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	currentFile = state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after second navigate")
	}

	// Toggle show again
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	currentFile = state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after toggle show")
	}
}

func TestFilterPlusHidden_UserScenario_ToggleShowFilterToggleHideToggleShow(t *testing.T) {
	// Scenario from user report:
	// 1. Start with home dirs (HideHiddenFiles=true by default)
	// 2. Press . (dot) to show hidden files
	// 3. Press / and search for letter "i" (finds both visible and hidden files)
	// 4. Cursor lands on first found file - which is hidden (.config, .hidden_x contain "i")
	// 5. Press . (dot) to hide hidden files - cursor should move to visible file or become invalid
	// 6. Press . (dot) again to show hidden - cursor should land somewhere valid

	state := &AppState{
		CurrentPath: "/home",
		Files: []FileEntry{
			{Name: "Desktop", IsDir: true},          // no "i"
			{Name: ".config", IsDir: true},          // has "i"
			{Name: "Downloads", IsDir: true},        // has "i"
			{Name: ".hidden_settings", IsDir: true}, // has "i"
			{Name: "Pictures", IsDir: true},         // has "i"
			{Name: ".hidden_items", IsDir: true},    // has "i"
		},
		HideHiddenFiles: true, // Start with hidden files hidden
		SelectedIndex:   0,    // On Desktop
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// STEP 1: Press . (dot) to show hidden files
	t.Logf("=== STEP 1: Toggle to show hidden files ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed at step 1: %v", err)
	}
	if state.HideHiddenFiles {
		t.Errorf("HideHiddenFiles should be false after toggle")
	}
	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL at step 1: currentFile is nil")
	} else if currentFile.IsHidden() {
		t.Errorf("FAIL at step 1: cursor should be on non-hidden file, got %s", currentFile.Name)
	}

	// STEP 2: Press / and search for "i" - will match multiple files (both visible and hidden)
	t.Logf("=== STEP 2: Start filter and search for 'i' ===")
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed at step 2: %v", err)
	}
	// Simulate typing "i"
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'i'}); err != nil {
		t.Fatalf("Failed at step 2 (typing): %v", err)
	}

	// At this point, filter results should include:
	// - .config (hidden, contains "i")
	// - Downloads (visible, contains "i")
	// - .hidden_settings (hidden, contains "i")
	// - Pictures (visible, contains "i")
	// - .hidden_items (hidden, contains "i")
	displayFiles := state.getDisplayFiles()
	t.Logf("Filter results for 'i': %d files", len(displayFiles))
	for i, f := range displayFiles {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	// Find first hidden file in results and move cursor there
	hiddenInResults := -1
	for i, f := range displayFiles {
		if f.IsHidden() {
			hiddenInResults = i
			break
		}
	}

	if hiddenInResults >= 0 {
		// Navigate to the first hidden file in results
		state.SelectedIndex = state.FilteredIndices[hiddenInResults]
		t.Logf("Cursor moved to first hidden in results: %s", state.getCurrentFile().Name)
	}

	// STEP 3: Press . (dot) to hide hidden files
	t.Logf("=== STEP 3: Toggle to hide hidden files (with filter active) ===")
	t.Logf("Before toggle: SelectedIndex=%d, FilterActive=%v, FilterQuery=%s", state.SelectedIndex, state.FilterActive, state.FilterQuery)
	displayFilesBeforeToggle := state.getDisplayFiles()
	t.Logf("Before toggle: Display has %d files", len(displayFilesBeforeToggle))

	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed at step 3: %v", err)
	}
	if !state.HideHiddenFiles {
		t.Errorf("HideHiddenFiles should be true after toggle")
	}

	t.Logf("After toggle: SelectedIndex=%d, FilterActive=%v", state.SelectedIndex, state.FilterActive)
	displayFilesAfterToggle := state.getDisplayFiles()
	t.Logf("After toggle: Display has %d files", len(displayFilesAfterToggle))
	for i, f := range displayFilesAfterToggle {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	// Cursor should not be on a hidden file now
	currentFile = state.getCurrentFile()
	t.Logf("Current file after toggle: %v", currentFile)
	if currentFile != nil && currentFile.IsHidden() {
		t.Errorf("FAIL at step 3: cursor should not be on hidden file, got %s", currentFile.Name)
	}

	// STEP 4: Press . (dot) again to show hidden files
	t.Logf("=== STEP 4: Toggle to show hidden files again ===")
	t.Logf("Before toggle: SelectedIndex=%d, HideHiddenFiles=%v", state.SelectedIndex, state.HideHiddenFiles)

	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed at step 4: %v", err)
	}
	if state.HideHiddenFiles {
		t.Errorf("HideHiddenFiles should be false after toggle")
	}

	t.Logf("After toggle: SelectedIndex=%d", state.SelectedIndex)
	displayFilesAfterToggle2 := state.getDisplayFiles()
	t.Logf("After toggle: Display has %d files", len(displayFilesAfterToggle2))
	for i, f := range displayFilesAfterToggle2 {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	// Cursor should be visible and on some file
	currentFile = state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL at step 4: currentFile is nil - cursor disappeared!")
	} else {
		t.Logf("Final cursor position: %s (hidden=%v)", currentFile.Name, currentFile.IsHidden())
	}

	// Verify that after all toggles with filter active, cursor is still valid
	if len(displayFilesAfterToggle2) == 0 {
		t.Errorf("FAIL at step 4: no display files available but cursor should point somewhere")
	}

	// Check if cursor is in current display (from filtered results)
	if currentFile != nil {
		found := false
		currentDisplay := state.getDisplayFiles()
		for _, f := range currentDisplay {
			if f.Name == currentFile.Name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("FAIL at step 4: Cursor is on %s but file not in current display!", currentFile.Name)
		} else {
			t.Logf("OK: Cursor is on %s which is in current display", currentFile.Name)
		}
	}

	// Based on user's report: after showing hidden again, cursor should be on
	// a file that makes sense (ideally first visible file from original list)
	// Currently cursor is on: %s (which may or may not be correct)
	if currentFile != nil && currentFile.IsHidden() {
		t.Logf("NOTE: Cursor landed on hidden file %s - user may have expected first non-hidden from original list", currentFile.Name)
	}
}

func TestFilterPlusHidden_CursorDisappears_WithFilterAndNonHiddenSelection(t *testing.T) {
	// USER REPORT - EXACT BUG:
	// 1. Show hidden files (. dot)
	// 2. Start filter /o (finds Applications, Downloads, .config, .oh-my-zsh, etc)
	// 3. Cursor on Applications (non-hidden, matches filter)
	// 4. Hide hidden files again (. dot)
	// 5. BUG: Cursor disappears! (SelectedIndex becomes invalid)
	//
	// Expected: Cursor should stay on Applications since it:
	//   - Is non-hidden
	//   - Matches filter "/o"
	//   - Is valid with HideHiddenFiles=true + FilterActive=true

	state := &AppState{
		CurrentPath: "/home/user",
		Files: []FileEntry{
			{Name: "Applications", IsDir: true},  // has "o" - NON-hidden
			{Name: ".config", IsDir: true},       // has "o" - hidden
			{Name: "Desktop", IsDir: true},       // NO "o" - NON-hidden
			{Name: ".oh-my-zsh", IsDir: true},    // has "o" - hidden
			{Name: "Documents", IsDir: true},     // NO "o" - NON-hidden
			{Name: ".hidden_tools", IsDir: true}, // has "o" - hidden
			{Name: "Downloads", IsDir: true},     // has "o" - NON-hidden
			{Name: ".dotfiles", IsDir: true},     // has "o" - hidden
		},
		HideHiddenFiles: true,
		SelectedIndex:   0, // On Applications
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// STEP 1: Show hidden files
	t.Logf("=== STEP 1: Show hidden files ===")
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if state.HideHiddenFiles {
		t.Errorf("HideHiddenFiles should be false")
	}
	if state.SelectedIndex != 0 {
		t.Errorf("Cursor should still be on Applications (index 0), got %d", state.SelectedIndex)
	}

	// STEP 2: User manually navigates to Applications (since cursor may move due to fuzzy sorting)
	t.Logf("=== STEP 2: Start filter for 'o' and navigate to Applications ===")
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'o'}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	displayFiles := state.getDisplayFiles()
	t.Logf("Filter results for 'o': %d files", len(displayFiles))
	for i, f := range displayFiles {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	// Find and navigate to Applications in the filtered results
	appIdx := -1
	for i, f := range state.Files {
		if f.Name == "Applications" {
			appIdx = i
			break
		}
	}
	if appIdx == -1 {
		t.Fatalf("Applications not found in Files")
	}

	// Set cursor to Applications
	state.SelectedIndex = appIdx
	currentFile := state.getCurrentFile()
	t.Logf("Cursor manually set to Applications: %v", currentFile)
	if currentFile == nil || currentFile.Name != "Applications" {
		t.Fatalf("Could not set cursor to Applications")
	}

	// STEP 3: Hide hidden files again (THIS IS WHERE BUG HAPPENS)
	t.Logf("=== STEP 3: Hide hidden files (BUG SCENARIO) ===")
	t.Logf("Before toggle: SelectedIndex=%d, FilterActive=%v", state.SelectedIndex, state.FilterActive)
	t.Logf("  Files[0]=%s, Files[%d]=Applications", state.Files[0].Name, 0)
	t.Logf("  FilteredIndices before: %v", state.FilteredIndices)

	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	if state.HideHiddenFiles != true {
		t.Errorf("HideHiddenFiles should be true")
	}

	t.Logf("After toggle: SelectedIndex=%d", state.SelectedIndex)
	t.Logf("  FilteredIndices after: %v", state.FilteredIndices)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("  Files[%d]=%s", state.SelectedIndex, state.Files[state.SelectedIndex].Name)
	}

	// CHECK IF CURSOR DISAPPEARED - THIS IS THE BUG
	displayFilesAfter := state.getDisplayFiles()
	t.Logf("Display after toggle: %d files", len(displayFilesAfter))
	for i, f := range displayFilesAfter {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	currentFileAfter := state.getCurrentFile()
	if currentFileAfter == nil {
		t.Errorf("BUG CONFIRMED: Cursor disappeared! (SelectedIndex=%d, FilterActive=%v)", state.SelectedIndex, state.FilterActive)
	} else {
		t.Logf("Cursor position after toggle: %s", currentFileAfter.Name)
		// Cursor should still be on Applications since it's non-hidden and matches "o"
		if currentFileAfter.Name != "Applications" {
			t.Errorf("Cursor should be on Applications, got %s", currentFileAfter.Name)
		}
	}
}

func TestFilterPlusHidden_FilterCursorPrefersPreviousVisible(t *testing.T) {
	// Legacy regression guard: when the inline filter is active, toggling the
	// hidden-files flag should keep the cursor on the closest still-visible entry
	// using the display order (which now matches the directory order).

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "sanctuary"}, // 0 - not hidden
			{Name: ".safari"},   // 1 - HIDDEN
			{Name: "sandbox"},   // 2 - not hidden
			{Name: ".scaffold"}, // 3 - HIDDEN
		},
		FilterActive: true,
		FilterQuery:  "sa", // IMPORTANT: Need non-empty query to avoid recomputeFilter clearing FilteredIndices
		// Filter results preserve directory order: sanctuary, .safari, sandbox, .scaffold
		FilteredIndices: []int{0, 1, 2, 3},
		FilterMatches: []FuzzyMatch{
			{FileIndex: 0, Score: 90},
			{FileIndex: 1, Score: 70},
			{FileIndex: 2, Score: 80},
			{FileIndex: 3, Score: 60},
		},
		HideHiddenFiles: false,
		SelectedIndex:   1, // .safari (displayIdx=1 while visible)
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	t.Logf("=== Before Toggle ===")
	t.Logf("SelectedIndex=%d, File=%s", state.SelectedIndex, state.Files[state.SelectedIndex].Name)
	displayIdxBefore := state.getDisplaySelectedIndex()
	t.Logf("displayIdx=%d in directory order", displayIdxBefore)
	displayFiles := state.getDisplayFiles()
	for i, f := range displayFiles {
		t.Logf("  [%d] %s (hidden=%v)", i, f.Name, f.IsHidden())
	}

	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	t.Logf("=== After Toggle Hide ===")
	t.Logf("SelectedIndex=%d", state.SelectedIndex)
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Files) {
		t.Logf("File=%s", state.Files[state.SelectedIndex].Name)
	}
	displayIdxAfter := state.getDisplaySelectedIndex()
	t.Logf("displayIdx=%d", displayIdxAfter)
	displayFilesAfter := state.getDisplayFiles()
	for i, f := range displayFilesAfter {
		t.Logf("  [%d] %s", i, f.Name)
	}

	// Expected behavior:
	// - .safari sat between sanctuary and sandbox in display order
	// - After hiding .safari, cursor should move to the nearest visible entry,
	//   preferring the previous row if there is a tie.
	currentFile := state.getCurrentFile()
	if currentFile == nil {
		t.Errorf("FAIL: currentFile is nil after toggle")
	} else if currentFile.Name != "sanctuary" {
		t.Errorf("FAIL: Expected sanctuary, got %s", currentFile.Name)
	} else {
		t.Logf("PASS: Cursor correctly selected previous visible entry")
	}
}

func TestFilterPlusHidden_FuzzySearchTwoHiddenFilesPosition(t *testing.T) {
	// USER REPORT - DEPENDS ON WHICH HIDDEN FILE SELECTED:
	// When multiple hidden files are in fuzzy results, which one is selected
	// affects where cursor lands after toggle hide.
	//
	// This should be consistent: cursor should always pick nearest visible
	// in display order, regardless of which hidden file was selected.
	//
	// Files: [apple, .alpha, .ant, banana]
	// Fuzzy "/a": Matches .alpha(.a), .ant(an), apple(a), apple(a)
	//   Sorted by score: apple(100), .alpha(90), .ant(80)
	// FilteredIndices = [0, 1, 2]
	// Display: [apple(0), .alpha(1), .ant(2)]

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple"},  // 0 - not hidden
			{Name: ".alpha"}, // 1 - HIDDEN
			{Name: ".ant"},   // 2 - HIDDEN
			{Name: "banana"}, // 3 - not hidden
		},
		FilterActive: true,
		FilterQuery:  "a", // IMPORTANT: Need non-empty query
		// Fuzzy sorted: apple(100), .alpha(90), .ant(80)
		FilteredIndices: []int{0, 1, 2},
		FilterMatches: []FuzzyMatch{
			{FileIndex: 0, Score: 100},
			{FileIndex: 1, Score: 90},
			{FileIndex: 2, Score: 80},
		},
		HideHiddenFiles: false,
		SelectedIndex:   1, // .alpha at displayIdx=1
		ScrollOffset:    0,
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Test with .alpha selected
	t.Logf("=== Test 1: .alpha selected ===")
	state.SelectedIndex = 1 // .alpha
	t.Logf("Before: SelectedIndex=%d (File=%s)", state.SelectedIndex, state.Files[state.SelectedIndex].Name)

	state1 := *state
	if _, err := reducer.Reduce(&state1, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	result1 := state1.getCurrentFile()
	t.Logf("After toggle: SelectedIndex=%d (File=%v)", state1.SelectedIndex, result1)

	// Test with .ant selected
	t.Logf("=== Test 2: .ant selected ===")
	state.SelectedIndex = 2 // .ant
	t.Logf("Before: SelectedIndex=%d (File=%s)", state.SelectedIndex, state.Files[state.SelectedIndex].Name)

	state2 := *state
	if _, err := reducer.Reduce(&state2, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	result2 := state2.getCurrentFile()
	t.Logf("After toggle: SelectedIndex=%d (File=%v)", state2.SelectedIndex, result2)

	// EXPECTED: Both should select apple (the only visible in filtered results)
	if result1 == nil {
		t.Errorf("Test 1 FAIL: result is nil")
	} else if result1.Name != "apple" {
		t.Errorf("Test 1 FAIL: Expected apple, got %s", result1.Name)
	} else {
		t.Logf("Test 1 PASS: Selected apple")
	}

	if result2 == nil {
		t.Errorf("Test 2 FAIL: result is nil")
	} else if result2.Name != "apple" {
		t.Errorf("Test 2 FAIL: Expected apple, got %s", result2.Name)
	} else {
		t.Logf("Test 2 PASS: Selected apple")
	}

	// Both tests should have the same result - this ensures stability
	if result1 != nil && result2 != nil && result1.Name != result2.Name {
		t.Errorf("FAIL: Inconsistent behavior - selected different files depending on which hidden was chosen: %s vs %s", result1.Name, result2.Name)
	} else if result1 != nil && result2 != nil {
		t.Logf("PASS: Consistent behavior - both selected %s", result1.Name)
	}
}
