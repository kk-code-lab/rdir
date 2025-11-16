package state

import (
	"strings"
	"testing"

	search "github.com/kk-code-lab/rdir/internal/search"
)

// ===== FILTER TESTS =====

func TestFilterStartActivation(t *testing.T) {
	state := &AppState{
		CurrentPath:  "/test",
		FilterActive: false,
		FilterQuery:  "",
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}

	if !state.FilterActive {
		t.Error("Filter should be active")
	}
	if state.FilterQuery != "" {
		t.Error("Filter query should be empty at start")
	}
}

func TestFilterStartKeepsSelection(t *testing.T) {
	// When "/" is pressed to start filter, selection should stay on the current row
	// so the highlight doesn't disappear before typing
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
			{Name: "file3.txt", IsDir: false},
		},
		SelectedIndex: 1, // Selected on file2.txt
		FilterActive:  false,
		FilterQuery:   "",
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	// Press "/" to activate filter
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}

	// Verify filter is active
	if !state.FilterActive {
		t.Error("Filter should be active")
	}

	// Verify selection is preserved
	if state.SelectedIndex != 1 {
		t.Errorf("Selection should remain on previously selected file (1), got %d", state.SelectedIndex)
	}

	// Verify saved index is preserved for restoration
	if state.FilterSavedIndex != 1 {
		t.Errorf("FilterSavedIndex should be 1 (the previous selection), got %d", state.FilterSavedIndex)
	}

	// Verify FilteredIndices is initialized
	if len(state.FilteredIndices) != 3 {
		t.Errorf("FilteredIndices should have 3 items, got %d", len(state.FilteredIndices))
	}

	t.Logf("Before NavigateDown: SelectedIndex=%d, DisplaySelectedIndex=%d, FilteredIndices=%v",
		state.SelectedIndex, state.getDisplaySelectedIndex(), state.FilteredIndices)

	// Verify that pressing down moves to the next file
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}

	t.Logf("After NavigateDown: SelectedIndex=%d, DisplaySelectedIndex=%d, FilteredIndices=%v",
		state.SelectedIndex, state.getDisplaySelectedIndex(), state.FilteredIndices)

	if state.SelectedIndex != 2 {
		t.Errorf("After pressing down, should advance to file at index 2, got %d", state.SelectedIndex)
	}
}

func TestMatchFilterTokensNonContiguousMultiTokens(t *testing.T) {
	matcher := search.NewFuzzyMatcher()
	tokens := prepareFilterTokens("fcl dsp", false)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	name := "root/project/docs/DSP/html/ftv2cl.png"
	score, matched := matchFilterTokens(name, strings.ToLower(name), tokens, false, matcher)
	if !matched {
		t.Fatalf("expected tokens to match %q, score=%.4f", name, score)
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %.4f", score)
	}
}

func TestFilterSpaceOnlyKeepsSelection(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "first.txt"},
			{Name: "second.txt"},
			{Name: "third.txt"},
		},
		SelectedIndex: 1,
		ScreenHeight:  30,
		ScreenWidth:   100,
	}
	reducer := NewStateReducer()

	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}
	if state.SelectedIndex != 1 {
		t.Fatalf("Expected selection to remain on index 1 after starting filter, got %d", state.SelectedIndex)
	}

	if _, err := reducer.Reduce(state, FilterCharAction{Char: ' '}); err != nil {
		t.Fatalf("Failed to append space: %v", err)
	}
	if state.SelectedIndex != 1 {
		t.Fatalf("Selection should stay on index 1 after typing space, got %d", state.SelectedIndex)
	}
	if len(state.FilteredIndices) != len(state.Files) {
		t.Fatalf("Space-only query should keep all files visible, got %d entries", len(state.FilteredIndices))
	}
}

func TestFilterStartWhenAlreadyActive(t *testing.T) {
	// Bug fix: When filter is already active and user presses "/" again,
	// FilteredIndices should be reset to [0, 1, 2, ...] (not keep old sorted order from previous search)
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple.txt", IsDir: false},
			{Name: "banana.txt", IsDir: false},
			{Name: "cherry.txt", IsDir: false},
		},
		FilterActive:  true,
		FilterQuery:   "ba", // User previously searched for "ba"
		SelectedIndex: 1,    // Currently on banana
		// FilteredIndices has been sorted by fuzzy match score (worst case: reverse order)
		FilteredIndices: []int{2, 1, 0}, // cherry, banana, apple (wrong order!)
		ScreenHeight:    24,
		ScreenWidth:     80,
	}

	reducer := NewStateReducer()

	// Press "/" again to reset filter while already in filter mode
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}

	// Verify FilteredIndices is reset to proper order
	if len(state.FilteredIndices) != 3 {
		t.Errorf("FilteredIndices should have 3 items, got %d", len(state.FilteredIndices))
	}

	if state.FilteredIndices[0] != 0 || state.FilteredIndices[1] != 1 || state.FilteredIndices[2] != 2 {
		t.Errorf("FilteredIndices should be [0 1 2], got %v", state.FilteredIndices)
	}

	// Verify selection is reset to -1
	if state.SelectedIndex != -1 {
		t.Errorf("SelectedIndex should be -1, got %d", state.SelectedIndex)
	}

	// Verify FilterQuery is cleared
	if state.FilterQuery != "" {
		t.Errorf("FilterQuery should be empty, got %q", state.FilterQuery)
	}

	// Now when pressing down, should select index 0 (apple), not index 2 (cherry)
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}
	if state.SelectedIndex != 0 {
		t.Errorf("After pressing down, should select file at index 0, got %d", state.SelectedIndex)
	}
}

func TestFilterResetQueryKeepsModeActive(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha.txt"},
			{Name: "beta.txt"},
		},
		FilterActive:    true,
		FilterQuery:     "al",
		FilteredIndices: []int{0},
		SelectedIndex:   0,
		ScrollOffset:    1,
	}

	reducer := NewStateReducer()

	// Seed cache before action to ensure reset invalidates it
	state.displayFilesCache = []FileEntry{{Name: "only"}}

	if _, err := reducer.Reduce(state, FilterResetQueryAction{}); err != nil {
		t.Fatalf("Failed to reset filter query: %v", err)
	}

	if !state.FilterActive {
		t.Fatal("Filter should remain active after reset")
	}
	if state.FilterQuery != "" {
		t.Fatalf("Filter query should be empty after reset, got %q", state.FilterQuery)
	}
	if len(state.FilteredIndices) != len(state.Files) {
		t.Fatalf("Filtered indices should include all files, got %v", state.FilteredIndices)
	}
	if state.SelectedIndex != 0 {
		t.Fatalf("SelectedIndex should remain on current file (0), got %d", state.SelectedIndex)
	}
	if state.ScrollOffset != 0 {
		t.Fatalf("ScrollOffset should be reset to 0, got %d", state.ScrollOffset)
	}

	display := state.getDisplayFiles()
	if len(display) != len(state.Files) {
		t.Fatalf("Display files should include all entries, got %d", len(display))
	}
}

func TestFilterClearFallbackSelection(t *testing.T) {
	state := &AppState{
		CurrentPath:      "/test",
		Files:            []FileEntry{{Name: "alpha"}, {Name: "beta"}},
		FilterActive:     true,
		FilterQuery:      "",
		SelectedIndex:    -1,
		FilterSavedIndex: 1,
	}

	reducer := NewStateReducer()

	if _, err := reducer.Reduce(state, FilterClearAction{}); err != nil {
		t.Fatalf("FilterClearAction failed: %v", err)
	}

	if state.FilterActive {
		t.Fatal("Filter should be inactive after clear")
	}
	if state.SelectedIndex != 1 {
		t.Fatalf("Expected SelectedIndex restored to saved value 1, got %d", state.SelectedIndex)
	}
}

func TestNavigateDownInFuzzyResults(t *testing.T) {
	// Bug: Press "/" → Type "r" → Press down
	// Should select FIRST file with "r", not second
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "CLAUDE.md", IsDir: false},         // No 'r'
			{Name: "FUZZY_SEARCH.md", IsDir: false},   // Has 'r' - FIRST match
			{Name: "IMPLEMENTATION.md", IsDir: false}, // Has 'r'
			{Name: "REFACTORING.md", IsDir: false},    // Has 'r'
			{Name: "rdir", IsDir: false},              // Starts with 'r'
		},
		SelectedIndex: 0,
		FilterActive:  false,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	// Step 1: Press "/"
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}
	if state.SelectedIndex != 0 {
		t.Errorf("After /, SelectedIndex should remain on current file (0), got %d", state.SelectedIndex)
	}

	// Step 2: Type "r"
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'r'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	t.Logf("After typing 'r':")
	t.Logf("  FilteredIndices: %v", state.FilteredIndices)
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)

	displayFiles := state.getDisplayFiles()
	for i, f := range displayFiles {
		t.Logf("  Display[%d]: %s", i, f.Name)
	}

	// Step 3: After typing "r", first match should be selected
	file := state.getCurrentFile()
	if file == nil {
		t.Error("getCurrentFile() returned nil after typing 'r'")
		return
	}

	t.Logf("After typing 'r', selected file: %s", file.Name)

	// After FilterCharAction, first match should be selected
	if file.Name != "REFACTORING.md" {
		t.Errorf("After typing 'r', expected first match (REFACTORING.md), got %s", file.Name)
	}

	// Step 4: Press down
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("Failed to navigate down: %v", err)
	}

	t.Logf("After pressing down:")
	t.Logf("  SelectedIndex: %d", state.SelectedIndex)
	t.Logf("  getDisplaySelectedIndex: %d", state.getDisplaySelectedIndex())

	file = state.getCurrentFile()
	if file == nil {
		t.Error("getCurrentFile() returned nil")
		return
	}

	t.Logf("  Selected file: %s", file.Name)

	// Should move to second match (rdir)
	if file.Name != "rdir" {
		t.Errorf("After pressing down, expected second match (rdir), got %s", file.Name)
	}
}

func TestFilterSpaceSeparatedTokensRequireAll(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha beta.txt"},
			{Name: "alpha.txt"},
			{Name: "beta alpha.md"},
			{Name: "beta.txt"},
		},
		FilterActive: true,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	state.FilterQuery = "alpha beta"
	state.recomputeFilter()

	if len(state.FilteredIndices) != 2 {
		t.Fatalf("Expected 2 files to match both tokens, got %d", len(state.FilteredIndices))
	}

	matched := map[string]bool{}
	for _, idx := range state.FilteredIndices {
		matched[state.Files[idx].Name] = true
	}
	if !matched["alpha beta.txt"] || !matched["beta alpha.md"] {
		t.Fatalf("Expected both 'alpha beta.txt' and 'beta alpha.md' to match, got %v", matched)
	}
}

func TestFilterTrailingSpaceIgnored(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha.md"},
			{Name: "alpha beta.md"},
			{Name: "beta.txt"},
		},
		FilterActive: true,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	state.FilterQuery = "alpha"
	state.recomputeFilter()
	initial := append([]int(nil), state.FilteredIndices...)

	state.FilterQuery = "alpha "
	state.recomputeFilter()

	if len(initial) != len(state.FilteredIndices) {
		t.Fatalf("Trailing space should not change match count (before %d, after %d)", len(initial), len(state.FilteredIndices))
	}
	for i := range initial {
		if initial[i] != state.FilteredIndices[i] {
			t.Fatalf("Trailing space should not change order (differs at %d: %v vs %v)", i, initial, state.FilteredIndices)
		}
	}
}

func TestEnterDirectoryFromFilteredViewDoesNotRestoreSavedIndex(t *testing.T) {
	// Bug fix: When entering directory from filtered view, SelectedIndex should be 0
	// NOT restored from previous visit to that directory
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple.txt", IsDir: false},
			{Name: "banana.txt", IsDir: false},
			{Name: "cherry.txt", IsDir: false},
			{Name: "foo", IsDir: true}, // Index 3 - will be selected
			{Name: "grape.txt", IsDir: false},
		},
		SelectedIndex: 0,
		FilterActive:  false,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	// Step 1: Activate filter and search for "f"
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'f'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	// Now "foo" should be selected (index 3 in Files)
	if state.SelectedIndex != 3 {
		t.Errorf("After filter 'f', SelectedIndex should be 3 (foo), got %d", state.SelectedIndex)
	}

	// Step 2: Pre-populate selectionHistory to simulate previous visit
	// This simulates: user was in /test/foo, had cursor on index 4, then left
	reducer.selectionHistory["/test/foo"] = 4

	// Step 3: Enter the directory from filter
	// We'll mock changeDirectory by manually setting state as if we entered
	oldFiles := state.Files
	state.Files = []FileEntry{
		{Name: ".hidden", IsDir: false},
		{Name: "a.txt", IsDir: false},
		{Name: "b.txt", IsDir: false},
		{Name: "c.txt", IsDir: false},
		{Name: "d.txt", IsDir: false}, // Index 4 - previously selected
	}
	state.CurrentPath = "/test/foo"
	state.sortFiles()
	state.resetViewport() // This calls clearFilter() which no longer restores!

	// IMPORTANT: Manually simulate what EnterDirectoryAction does
	// It should check selectionHistory but only if NOT filtered
	wasFiltered := true // We were in filtered view!
	if !wasFiltered {
		if savedIdx, ok := reducer.selectionHistory["/test/foo"]; ok && savedIdx < len(state.Files) {
			state.SelectedIndex = savedIdx
		}
	}

	// After entering from filter, SelectedIndex should still be 0
	if state.SelectedIndex != 0 {
		t.Errorf("After entering directory from filter, SelectedIndex should be 0, got %d", state.SelectedIndex)
	}

	// Restore
	state.Files = oldFiles
}

func TestFilterCharAppend(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "main.go", IsDir: false},
			{Name: "test.go", IsDir: false},
			{Name: "readme.txt", IsDir: false},
		},
		FilterActive: true,
		FilterQuery:  "",
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'g'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'o'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	if state.FilterQuery != "go" {
		t.Errorf("Expected query 'go', got %q", state.FilterQuery)
	}

	// Should have filtered results
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 2 {
		t.Errorf("Expected 2 filtered files, got %d", len(displayFiles))
	}

	// Check they are main.go and test.go
	names := map[string]bool{}
	for _, f := range displayFiles {
		names[f.Name] = true
	}

	if !names["main.go"] || !names["test.go"] {
		t.Error("Filtered results should contain main.go and test.go")
	}
}

func TestFilterBackspace(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "main.go", IsDir: false},
			{Name: "test.go", IsDir: false},
		},
		FilterActive: true,
		FilterQuery:  "go",
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace: %v", err)
	}

	if state.FilterQuery != "g" {
		t.Errorf("Expected query 'g', got %q", state.FilterQuery)
	}
}

func TestFilterBackspaceLastCharStaysInFilterMode(t *testing.T) {
	// UX improvement: When backspacing to empty query, should stay in filter mode
	// instead of exiting (Esc is for explicitly exiting)
	// This matches behavior of FilterStartAction which has empty query + FilterActive=true

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple", IsDir: false},
			{Name: "banana", IsDir: false},
			{Name: "cherry", IsDir: false},
		},
		FilterActive:  true,
		FilterQuery:   "a", // Single character
		SelectedIndex: 0,   // On first match
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	t.Logf("=== Before backspace ===")
	t.Logf("FilterActive=%v, FilterQuery=%q, SelectedIndex=%d", state.FilterActive, state.FilterQuery, state.SelectedIndex)

	// Backspace the last character
	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace: %v", err)
	}

	t.Logf("=== After backspace ===")
	t.Logf("FilterActive=%v, FilterQuery=%q, SelectedIndex=%d", state.FilterActive, state.FilterQuery, state.SelectedIndex)

	// IMPORTANT: Filter should remain active
	if !state.FilterActive {
		t.Errorf("FAIL: FilterActive should be true after backspacing to empty, got false")
	}

	// Query should be empty
	if state.FilterQuery != "" {
		t.Errorf("FAIL: FilterQuery should be empty, got %q", state.FilterQuery)
	}

	// FilteredIndices should show all files (like FilterStartAction)
	if len(state.FilteredIndices) != 3 {
		t.Errorf("FAIL: FilteredIndices should have 3 files, got %d", len(state.FilteredIndices))
	}

	// SelectedIndex should stay on the previously selected entry to avoid cursor jumps
	if state.SelectedIndex != 0 {
		t.Errorf("FAIL: SelectedIndex should stay on prior selection (0), got %d", state.SelectedIndex)
	}

	// Display should show all files
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 3 {
		t.Errorf("FAIL: Display should show 3 files, got %d", len(displayFiles))
	}

	t.Logf("SUCCESS: Stayed in filter mode with all files visible")
}

func TestFilterClear(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "main.go", IsDir: false},
		},
		FilterActive:  true,
		FilterQuery:   "go",
		SelectedIndex: 0,
		ScrollOffset:  1,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, FilterClearAction{}); err != nil {
		t.Fatalf("Failed to clear filter: %v", err)
	}

	if state.FilterActive {
		t.Error("Filter should be inactive")
	}
	if state.FilterQuery != "" {
		t.Error("Filter query should be empty")
	}
	if state.SelectedIndex != 0 {
		t.Error("Selected should reset to 0")
	}
	if state.ScrollOffset != 0 {
		t.Error("Scroll offset should reset to 0")
	}
}

func TestFilterEmptyQuery(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "file1.txt", IsDir: false},
			{Name: "file2.txt", IsDir: false},
		},
		FilterActive: false,
		FilterQuery:  "",
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	// Apply filter with non-matching query
	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'x'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'y'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'z'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 0 {
		t.Errorf("Expected 0 results for non-matching filter, got %d", len(displayFiles))
	}
}

func TestFilterBackspaceFromNoMatchesToMatches(t *testing.T) {
	// USER REPORT - Cursor disappears edge case:
	// 1. Type "ap" -> matches found (apple, apricot) -> cursor shown
	// 2. Type "xyz" -> "apxyz" no matches -> cursor hidden (SelectedIndex = -1)
	// 3. Backspace -> "apxy" still no matches
	// 4. Backspace -> "apx" still no matches
	// 5. Backspace -> "ap" matches found again -> cursor should reappear
	// BUG: Cursor didn't reappear, had to press down/up

	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "apple", IsDir: false},
			{Name: "apricot", IsDir: false},
			{Name: "banana", IsDir: false},
			{Name: "cherry", IsDir: false},
		},
		FilterActive: true,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()

	// Step 1: Type "ap" -> matches found
	t.Logf("=== Step 1: Type 'ap' ===")
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'a'}); err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'p'}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	displayFiles := state.getDisplayFiles()
	t.Logf("After 'ap': FilterQuery=%q, matches=%d, SelectedIndex=%d", state.FilterQuery, len(displayFiles), state.SelectedIndex)

	if len(displayFiles) == 0 {
		t.Errorf("Expected matches for 'ap', got 0")
	}
	if state.SelectedIndex < 0 {
		t.Errorf("Expected cursor on match, got SelectedIndex=%d", state.SelectedIndex)
	}

	// Step 2: Type "xyz" -> "apxyz" no matches
	t.Logf("=== Step 2: Type 'xyz' ===")
	for _, ch := range "xyz" {
		if _, err := reducer.Reduce(state, FilterCharAction{Char: ch}); err != nil {
			t.Fatalf("Failed: %v", err)
		}
	}

	displayFiles = state.getDisplayFiles()
	t.Logf("After 'apxyz': FilterQuery=%q, matches=%d, SelectedIndex=%d", state.FilterQuery, len(displayFiles), state.SelectedIndex)

	if len(displayFiles) != 0 {
		t.Errorf("Expected no matches for 'apxyz', got %d", len(displayFiles))
	}
	if state.SelectedIndex != -1 {
		t.Errorf("Expected no cursor when no matches, got SelectedIndex=%d", state.SelectedIndex)
	}

	// Step 3: Backspace 'z' -> "apxy" still no matches
	t.Logf("=== Step 3: Backspace once ===")
	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	displayFiles = state.getDisplayFiles()
	t.Logf("After backspace to 'apxy': FilterQuery=%q, matches=%d, SelectedIndex=%d", state.FilterQuery, len(displayFiles), state.SelectedIndex)

	if len(displayFiles) != 0 {
		t.Errorf("Expected no matches for 'apxy', got %d", len(displayFiles))
	}

	// Step 4: Backspace 'y' -> "apx" still no matches
	t.Logf("=== Step 4: Backspace again ===")
	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	displayFiles = state.getDisplayFiles()
	t.Logf("After backspace to 'apx': FilterQuery=%q, matches=%d, SelectedIndex=%d", state.FilterQuery, len(displayFiles), state.SelectedIndex)

	// Step 5: Backspace 'x' -> "ap" matches found -> CURSOR SHOULD REAPPEAR
	t.Logf("=== Step 5: Backspace again -> should get matches and cursor ===")
	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("Failed: %v", err)
	}

	displayFiles = state.getDisplayFiles()
	t.Logf("After backspace to 'ap': FilterQuery=%q, matches=%d, SelectedIndex=%d", state.FilterQuery, len(displayFiles), state.SelectedIndex)

	if len(displayFiles) == 0 {
		t.Errorf("FAIL: Expected matches for 'ap', got 0")
	}

	// THIS IS THE KEY CHECK: cursor should appear
	if state.SelectedIndex < 0 {
		t.Errorf("FAIL: Cursor should reappear when matches return, got SelectedIndex=%d", state.SelectedIndex)
	}

	// Cursor should be on first match
	file := state.getCurrentFile()
	if file == nil {
		t.Errorf("FAIL: getCurrentFile() returned nil - cursor not positioned correctly")
	} else {
		t.Logf("SUCCESS: Cursor on %s (SelectedIndex=%d)", file.Name, state.SelectedIndex)
		// Should be on apple or apricot
		if file.Name != "apple" && file.Name != "apricot" {
			t.Errorf("FAIL: Expected cursor on apple or apricot, got %s", file.Name)
		}
	}
}
