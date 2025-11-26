package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	searchpkg "github.com/kk-code-lab/rdir/internal/search"
)

// ===== GLOBAL SEARCH TESTS =====

func TestGlobalSearchStartAction(t *testing.T) {
	state := &AppState{
		CurrentPath:        "/test",
		GlobalSearchActive: false,
		GlobalSearchQuery:  "",
		GlobalSearchIndex:  0,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, GlobalSearchStartAction{}); err != nil {
		t.Fatalf("Failed to start global search: %v", err)
	}

	if !state.GlobalSearchActive {
		t.Error("GlobalSearch should be active")
	}
	if state.GlobalSearchQuery != "" {
		t.Error("GlobalSearch query should be empty at start")
	}
	if state.GlobalSearchRootPath != "/test" {
		t.Errorf("GlobalSearch root path should be /test, got %s", state.GlobalSearchRootPath)
	}
}

func TestGlobalSearchStartActionRestoresQueryInSameRoot(t *testing.T) {
	state := &AppState{
		CurrentPath:                   "/test",
		LastGlobalSearchQuery:         "fooBar",
		LastGlobalSearchRootPath:      "/test",
		LastGlobalSearchIndex:         5,
		LastGlobalSearchScroll:        2,
		LastGlobalSearchSelectionPath: "/test/fooBar.go",
		GlobalSearchCursorPos:         0,
		GlobalSearchCaseSensitive:     false,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, GlobalSearchStartAction{}); err != nil {
		t.Fatalf("Failed to start global search: %v", err)
	}

	if state.GlobalSearchQuery != "fooBar" {
		t.Fatalf("Expected query to be restored to 'fooBar', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != len([]rune("fooBar")) {
		t.Fatalf("Cursor should jump to end of restored query, got %d", state.GlobalSearchCursorPos)
	}
	if !state.GlobalSearchCaseSensitive {
		t.Fatal("Case sensitivity should be enabled when restored query has uppercase characters")
	}
	if state.GlobalSearchIndex != 5 {
		t.Fatalf("Expected selection index 5 to be restored, got %d", state.GlobalSearchIndex)
	}
	if state.GlobalSearchScroll != 2 {
		t.Fatalf("Expected scroll position 2 to be restored, got %d", state.GlobalSearchScroll)
	}
	if state.GlobalSearchDesiredSelectionPath != "/test/fooBar.go" {
		t.Fatalf("Expected desired selection path to be '/test/fooBar.go', got '%s'", state.GlobalSearchDesiredSelectionPath)
	}
	if !state.GlobalSearchPendingIndexActive || state.GlobalSearchPendingIndex != 5 {
		t.Fatalf("Expected pending index to be active with value 5, got active=%v value=%d", state.GlobalSearchPendingIndexActive, state.GlobalSearchPendingIndex)
	}
}

func TestGlobalSearchStartActionSkipsRestoreInDifferentRoot(t *testing.T) {
	state := &AppState{
		CurrentPath:              "/other",
		LastGlobalSearchQuery:    "foo",
		LastGlobalSearchRootPath: "/test",
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, GlobalSearchStartAction{}); err != nil {
		t.Fatalf("Failed to start global search: %v", err)
	}

	if state.GlobalSearchQuery != "" {
		t.Fatalf("Query should be empty when roots do not match, got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Fatalf("Cursor should remain at 0 when not restoring query, got %d", state.GlobalSearchCursorPos)
	}
	if state.GlobalSearchCaseSensitive {
		t.Fatal("Case sensitivity should remain disabled when no query is restored")
	}
}

func TestGlobalSearchDesiredSelectionRestoredWhenResultAppears(t *testing.T) {
	state := &AppState{
		CurrentPath:                      "/tmp",
		GlobalSearchActive:               true,
		GlobalSearchIndex:                5,
		GlobalSearchScroll:               0,
		GlobalSearchDesiredSelectionPath: "/tmp/target.txt",
		ScreenHeight:                     24,
	}

	reducer := NewStateReducer()

	firstBatch := []GlobalSearchResult{
		{FilePath: "/tmp/a.txt", FileName: "a.txt", DirPath: "/tmp"},
		{FilePath: "/tmp/b.txt", FileName: "b.txt", DirPath: "/tmp"},
	}

	_, err := reducer.Reduce(state, GlobalSearchResultsAction{
		Results:    firstBatch,
		InProgress: true,
		Phase:      SearchStatusWalking,
	})
	if err != nil {
		t.Fatalf("First results action failed: %v", err)
	}
	if state.GlobalSearchDesiredSelectionPath != "/tmp/target.txt" {
		t.Fatalf("Desired selection should remain pending until result appears, got '%s'", state.GlobalSearchDesiredSelectionPath)
	}

	secondBatch := append(firstBatch, GlobalSearchResult{
		FilePath: "/tmp/target.txt", FileName: "target.txt", DirPath: "/tmp",
	})

	_, err = reducer.Reduce(state, GlobalSearchResultsAction{
		Results:    secondBatch,
		InProgress: false,
		Phase:      SearchStatusComplete,
	})
	if err != nil {
		t.Fatalf("Second results action failed: %v", err)
	}

	if state.GlobalSearchIndex != 2 {
		t.Fatalf("Expected selection to jump to target index 2, got %d", state.GlobalSearchIndex)
	}
	if state.GlobalSearchDesiredSelectionPath != "" {
		t.Fatalf("Desired selection should clear after selecting target, got '%s'", state.GlobalSearchDesiredSelectionPath)
	}
	if state.GlobalSearchPendingIndexActive {
		t.Fatalf("Pending index should clear after selecting target, got active=%v value=%d", state.GlobalSearchPendingIndexActive, state.GlobalSearchPendingIndex)
	}
}

func TestGlobalSearchPendingIndexAdvancesAsResultsGrow(t *testing.T) {
	state := &AppState{
		GlobalSearchActive:             true,
		GlobalSearchPendingIndex:       4,
		GlobalSearchPendingIndexActive: true,
		GlobalSearchIndex:              0,
		GlobalSearchScroll:             0,
		ScreenHeight:                   20,
	}

	reducer := NewStateReducer()

	firstBatch := []GlobalSearchResult{
		{FilePath: "/tmp/a"},
		{FilePath: "/tmp/b"},
	}

	_, err := reducer.Reduce(state, GlobalSearchResultsAction{
		Results:    firstBatch,
		InProgress: true,
		Phase:      SearchStatusWalking,
	})
	if err != nil {
		t.Fatalf("First results action failed: %v", err)
	}

	if !state.GlobalSearchPendingIndexActive || state.GlobalSearchPendingIndex != 4 {
		t.Fatalf("Pending index should remain active at 4, got active=%v value=%d", state.GlobalSearchPendingIndexActive, state.GlobalSearchPendingIndex)
	}

	secondBatch := append(firstBatch,
		GlobalSearchResult{FilePath: "/tmp/c"},
		GlobalSearchResult{FilePath: "/tmp/d"},
		GlobalSearchResult{FilePath: "/tmp/e"},
		GlobalSearchResult{FilePath: "/tmp/f"},
	)

	_, err = reducer.Reduce(state, GlobalSearchResultsAction{
		Results:    secondBatch,
		InProgress: false,
		Phase:      SearchStatusComplete,
	})
	if err != nil {
		t.Fatalf("Second results action failed: %v", err)
	}

	if state.GlobalSearchIndex != 4 {
		t.Fatalf("Expected selection to jump to pending index 4, got %d", state.GlobalSearchIndex)
	}
	if state.GlobalSearchPendingIndexActive {
		t.Fatalf("Pending index should clear after applying, got active=%v value=%d", state.GlobalSearchPendingIndexActive, state.GlobalSearchPendingIndex)
	}
}

func TestGlobalSearchCharAction(t *testing.T) {
	state := &AppState{
		CurrentPath:           "/test",
		GlobalSearchActive:    true,
		GlobalSearchQuery:     "",
		GlobalSearchCursorPos: 0,
		GlobalSearchIndex:     0,
		GlobalSearchResults:   []GlobalSearchResult{},
	}

	reducer := NewStateReducer()

	// Type 'f'
	if _, err := reducer.Reduce(state, GlobalSearchCharAction{Char: 'f'}); err != nil {
		t.Fatalf("Failed to add char to global search: %v", err)
	}

	if state.GlobalSearchQuery != "f" {
		t.Errorf("GlobalSearch query should be 'f', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 1 {
		t.Errorf("Cursor position should advance to 1, got %d", state.GlobalSearchCursorPos)
	}
	if state.GlobalSearchIndex != 0 {
		t.Errorf("GlobalSearch index should be reset to 0, got %d", state.GlobalSearchIndex)
	}

	// Type 'o'
	if _, err := reducer.Reduce(state, GlobalSearchCharAction{Char: 'o'}); err != nil {
		t.Fatalf("Failed to add char to global search: %v", err)
	}

	if state.GlobalSearchQuery != "fo" {
		t.Errorf("GlobalSearch query should be 'fo', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 2 {
		t.Errorf("Cursor position should advance to 2, got %d", state.GlobalSearchCursorPos)
	}
}

func TestGlobalSearchBackspaceAction(t *testing.T) {
	state := &AppState{
		CurrentPath:           "/test",
		GlobalSearchActive:    true,
		GlobalSearchQuery:     "foo",
		GlobalSearchCursorPos: 3,
		GlobalSearchIndex:     0,
		GlobalSearchResults:   []GlobalSearchResult{},
	}

	reducer := NewStateReducer()

	// Press backspace once
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace in global search: %v", err)
	}

	if state.GlobalSearchQuery != "fo" {
		t.Errorf("GlobalSearch query should be 'fo', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 2 {
		t.Errorf("Cursor position should move to 2, got %d", state.GlobalSearchCursorPos)
	}

	// Press backspace until empty
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace: %v", err)
	}
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace: %v", err)
	}

	if state.GlobalSearchQuery != "" {
		t.Errorf("GlobalSearch query should be empty, got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor position should be 0 after deleting all characters, got %d", state.GlobalSearchCursorPos)
	}

	// Backspace on empty should be no-op
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Failed on empty backspace: %v", err)
	}

	if state.GlobalSearchQuery != "" {
		t.Errorf("GlobalSearch query should still be empty")
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor position should remain at 0 when no characters are deleted")
	}

	// Deleting character before the cursor should move the cursor left
	state.setGlobalSearchQuery("bar")
	state.GlobalSearchCursorPos = 2 // cursor between 'a' and 'r'
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Failed to backspace from middle: %v", err)
	}
	if state.GlobalSearchQuery != "br" {
		t.Errorf("Expected query to be 'br', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 1 {
		t.Errorf("Cursor should move to index 1 after deleting middle char, got %d", state.GlobalSearchCursorPos)
	}

	// Backspace at cursor 0 should be a no-op
	state.GlobalSearchCursorPos = 0
	prevQuery := state.GlobalSearchQuery
	if _, err := reducer.Reduce(state, GlobalSearchBackspaceAction{}); err != nil {
		t.Fatalf("Backspace at start should not error: %v", err)
	}
	if state.GlobalSearchQuery != prevQuery {
		t.Errorf("Query should remain '%s', got '%s'", prevQuery, state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should remain at start when no character deleted, got %d", state.GlobalSearchCursorPos)
	}
}

func TestGlobalSearchDeleteAction(t *testing.T) {
	state := &AppState{
		CurrentPath:               "/test",
		GlobalSearchActive:        true,
		GlobalSearchQuery:         "foobar",
		GlobalSearchCursorPos:     3, // cursor between "foo" and "bar"
		GlobalSearchResults:       []GlobalSearchResult{},
		GlobalSearchRootPath:      "/test",
		GlobalSearchID:            1,
		GlobalSearchCaseSensitive: false,
	}

	reducer := NewStateReducer()

	prevID := state.GlobalSearchID
	if _, err := reducer.Reduce(state, GlobalSearchDeleteAction{}); err != nil {
		t.Fatalf("Failed to delete character in global search: %v", err)
	}

	if state.GlobalSearchQuery != "fooar" {
		t.Errorf("Expected query to be 'fooar', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 3 {
		t.Errorf("Cursor position should remain at 3 after delete, got %d", state.GlobalSearchCursorPos)
	}
	if state.GlobalSearchID == prevID {
		t.Errorf("GlobalSearchID should increment after delete")
	}

	// Delete at end should be a no-op
	state.GlobalSearchCursorPos = len([]rune(state.GlobalSearchQuery))
	prevID = state.GlobalSearchID
	if _, err := reducer.Reduce(state, GlobalSearchDeleteAction{}); err != nil {
		t.Fatalf("Delete at end should not error: %v", err)
	}
	if state.GlobalSearchQuery != "fooar" {
		t.Errorf("Query should remain 'fooar', got '%s'", state.GlobalSearchQuery)
	}
	if state.GlobalSearchID != prevID {
		t.Errorf("GlobalSearchID should not change when nothing is deleted")
	}
}

func TestApplyLocalSearchPreviewFiltersExtension(t *testing.T) {
	reducer := NewStateReducer()
	prevResults := []GlobalSearchResult{
		{FilePath: "/tmp/foo.txt", FileName: "foo.txt"},
		{FilePath: "/tmp/foobar.txt", FileName: "foobar.txt"},
		{FilePath: "/tmp/bar.txt", FileName: "bar.txt"},
	}
	state := &AppState{
		GlobalSearchActive:        true,
		GlobalSearchResults:       prevResults,
		GlobalSearchCaseSensitive: false,
		ScreenHeight:              24,
	}
	prevQuery := "foo"
	state.setGlobalSearchQuery("foob")

	reducer.applyLocalSearchPreview(state, prevResults, prevQuery)

	if len(state.GlobalSearchResults) != 1 || state.GlobalSearchResults[0].FilePath != "/tmp/foobar.txt" {
		t.Fatalf("expected preview to keep only foobar.txt, got %#v", state.GlobalSearchResults)
	}
}

func TestApplyLocalSearchPreviewUsesCachedResults(t *testing.T) {
	reducer := NewStateReducer()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "alpha.txt"))

	searcher := searchpkg.NewGlobalSearcher(root, false, nil)
	results := searcher.SearchRecursive("alpha", false)
	if len(results) == 0 {
		t.Fatalf("expected search results")
	}

	state := &AppState{
		GlobalSearchActive:        true,
		GlobalSearchCaseSensitive: false,
		GlobalSearcher:            searcher,
		ScreenHeight:              24,
		GlobalSearchRootPath:      root,
	}
	state.setGlobalSearchQuery("alpha")

	reducer.applyLocalSearchPreview(state, nil, "")

	if len(state.GlobalSearchResults) == 0 {
		t.Fatalf("expected cached results to be applied")
	}
	if state.GlobalSearchResults[0].FileName != "alpha.txt" {
		t.Fatalf("expected alpha.txt, got %#v", state.GlobalSearchResults[0])
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestGlobalSearchMoveCursorAction(t *testing.T) {
	state := &AppState{
		CurrentPath:           "/test",
		GlobalSearchActive:    true,
		GlobalSearchQuery:     "foo",
		GlobalSearchCursorPos: 1,
	}

	reducer := NewStateReducer()

	// Move left
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "left"}); err != nil {
		t.Fatalf("Failed to move cursor left: %v", err)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should be at 0 after moving left, got %d", state.GlobalSearchCursorPos)
	}

	// Moving left at start should not change position
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "left"}); err != nil {
		t.Fatalf("Failed to move cursor left at start: %v", err)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should stay at 0 when moving left at start, got %d", state.GlobalSearchCursorPos)
	}

	// Move right twice
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "right"}); err != nil {
		t.Fatalf("Failed to move cursor right: %v", err)
	}
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "right"}); err != nil {
		t.Fatalf("Failed to move cursor right second time: %v", err)
	}
	if state.GlobalSearchCursorPos != 2 {
		t.Errorf("Cursor should be at 2 after moving right twice, got %d", state.GlobalSearchCursorPos)
	}

	// Home should jump to start
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "home"}); err != nil {
		t.Fatalf("Failed to move cursor home: %v", err)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should be at start after home, got %d", state.GlobalSearchCursorPos)
	}

	// End should jump to length
	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "end"}); err != nil {
		t.Fatalf("Failed to move cursor end: %v", err)
	}
	if state.GlobalSearchCursorPos != len([]rune(state.GlobalSearchQuery)) {
		t.Errorf("Cursor should be at end after end key, got %d", state.GlobalSearchCursorPos)
	}

	// Word navigation should respect boundaries
	state.setGlobalSearchQuery("foo bar/baz")
	state.GlobalSearchCursorPos = len([]rune(state.GlobalSearchQuery))

	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "word-left"}); err != nil {
		t.Fatalf("Failed to move cursor word-left: %v", err)
	}
	if state.GlobalSearchCursorPos != 8 {
		t.Errorf("Expected cursor at start of 'baz' (8), got %d", state.GlobalSearchCursorPos)
	}

	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "word-left"}); err != nil {
		t.Fatalf("Failed to move cursor word-left again: %v", err)
	}
	if state.GlobalSearchCursorPos != 4 {
		t.Errorf("Expected cursor at start of 'bar' (4), got %d", state.GlobalSearchCursorPos)
	}

	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "word-left"}); err != nil {
		t.Fatalf("Failed to move cursor word-left to start: %v", err)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Expected cursor at start after multiple word-left, got %d", state.GlobalSearchCursorPos)
	}

	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "word-right"}); err != nil {
		t.Fatalf("Failed to move cursor word-right: %v", err)
	}
	if state.GlobalSearchCursorPos != 3 {
		t.Errorf("Expected cursor after 'foo' (3), got %d", state.GlobalSearchCursorPos)
	}

	if _, err := reducer.Reduce(state, GlobalSearchMoveCursorAction{Direction: "word-right"}); err != nil {
		t.Fatalf("Failed to move cursor word-right second time: %v", err)
	}
	if state.GlobalSearchCursorPos != 7 {
		t.Errorf("Expected cursor after 'bar/' (7), got %d", state.GlobalSearchCursorPos)
	}
}

func TestGlobalSearchResultsSelectionPersists(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		GlobalSearchActive: true,
		GlobalSearchIndex:  1,
	}

	action := GlobalSearchResultsAction{
		Results: []GlobalSearchResult{
			makeGSResult("/tmp/c.txt"),
			makeGSResult("/tmp/b.txt"),
			makeGSResult("/tmp/d.txt"),
		},
		InProgress: false,
		Phase:      SearchStatusComplete,
	}

	if _, err := reducer.Reduce(state, action); err != nil {
		t.Fatalf("GlobalSearchResultsAction failed: %v", err)
	}

	if state.GlobalSearchIndex != 1 {
		t.Fatalf("Expected selection to stay on same index 1, got %d", state.GlobalSearchIndex)
	}
}

func TestGlobalSearchResultsSelectionFallback(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		GlobalSearchActive: true,
		GlobalSearchIndex:  2,
	}

	action := GlobalSearchResultsAction{
		Results: []GlobalSearchResult{
			makeGSResult("/tmp/a.txt"),
			makeGSResult("/tmp/b.txt"),
		},
		InProgress: false,
		Phase:      SearchStatusComplete,
	}

	if _, err := reducer.Reduce(state, action); err != nil {
		t.Fatalf("GlobalSearchResultsAction failed: %v", err)
	}

	if state.GlobalSearchIndex != 1 {
		t.Fatalf("Expected fallback to nearest valid index 1, got %d", state.GlobalSearchIndex)
	}
}

func TestGlobalSearchScrollFollowsSelection(t *testing.T) {
	reducer := NewStateReducer()
	results := make([]GlobalSearchResult, 0, 30)
	for i := 0; i < 30; i++ {
		path := filepath.Join("/tmp", fmt.Sprintf("file_%02d.txt", i))
		results = append(results, makeGSResult(path))
	}

	state := &AppState{
		GlobalSearchActive:  true,
		GlobalSearchResults: results,
		GlobalSearchIndex:   0,
		ScreenHeight:        14, // visibleLines = 10
	}
	state.updateGlobalSearchScroll()

	for i := 0; i < 15; i++ {
		if _, err := reducer.Reduce(state, GlobalSearchNavigateAction{Direction: "down"}); err != nil {
			t.Fatalf("navigate down: %v", err)
		}
	}

	if state.GlobalSearchIndex != 15 {
		t.Fatalf("expected index 15, got %d", state.GlobalSearchIndex)
	}

	visibleLines := state.visibleLines()
	expectedScroll := state.GlobalSearchIndex - visibleLines + 1
	if expectedScroll < 0 {
		expectedScroll = 0
	}
	if state.GlobalSearchScroll != expectedScroll {
		t.Fatalf("expected scroll %d, got %d", expectedScroll, state.GlobalSearchScroll)
	}

	// Navigate back up and ensure scroll follows
	for i := 0; i < 5; i++ {
		if _, err := reducer.Reduce(state, GlobalSearchNavigateAction{Direction: "up"}); err != nil {
			t.Fatalf("navigate up: %v", err)
		}
	}
	if state.GlobalSearchIndex != 10 {
		t.Fatalf("expected index 10, got %d", state.GlobalSearchIndex)
	}
	if state.GlobalSearchScroll != expectedScroll {
		t.Fatalf("expected scroll to remain %d after moving up within window, got %d", expectedScroll, state.GlobalSearchScroll)
	}
}

func makeGSResult(path string) GlobalSearchResult {
	return GlobalSearchResult{
		FilePath: path,
		FileName: filepath.Base(path),
		DirPath:  filepath.Dir(path),
		FileEntry: FileEntry{
			Name:     filepath.Base(path),
			FullPath: path,
		},
	}
}

func TestGlobalSearchDeleteWordAction(t *testing.T) {
	state := &AppState{
		CurrentPath:           "/test",
		GlobalSearchActive:    true,
		GlobalSearchQuery:     "foo bar baz",
		GlobalSearchCursorPos: len([]rune("foo bar baz")),
	}

	reducer := NewStateReducer()

	if _, err := reducer.Reduce(state, GlobalSearchDeleteWordAction{}); err != nil {
		t.Fatalf("Failed to delete word: %v", err)
	}
	if state.GlobalSearchQuery != "foo bar " {
		t.Errorf("Expected query 'foo bar ', got %q", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != len([]rune("foo bar ")) {
		t.Errorf("Cursor should move to position %d, got %d", len([]rune("foo bar ")), state.GlobalSearchCursorPos)
	}

	// Delete another word (the space should remain at tail)
	if _, err := reducer.Reduce(state, GlobalSearchDeleteWordAction{}); err != nil {
		t.Fatalf("Failed to delete second word: %v", err)
	}
	if state.GlobalSearchQuery != "foo " {
		t.Errorf("Expected query 'foo ', got %q", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != len([]rune("foo ")) {
		t.Errorf("Cursor should be at end of 'foo ', got %d", state.GlobalSearchCursorPos)
	}

	// Deleting the final word should clear the query
	if _, err := reducer.Reduce(state, GlobalSearchDeleteWordAction{}); err != nil {
		t.Fatalf("Delete final word should not error: %v", err)
	}
	if state.GlobalSearchQuery != "" {
		t.Errorf("Expected query to be empty, got %q", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should reset to 0, got %d", state.GlobalSearchCursorPos)
	}

	// Deleting when already empty should be noop
	if _, err := reducer.Reduce(state, GlobalSearchDeleteWordAction{}); err != nil {
		t.Fatalf("Delete on empty should not error: %v", err)
	}
	if state.GlobalSearchQuery != "" {
		t.Errorf("Query should stay empty, got %q", state.GlobalSearchQuery)
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor should remain at 0, got %d", state.GlobalSearchCursorPos)
	}
}

func TestGlobalSearchResetQueryAction(t *testing.T) {
	state := &AppState{
		CurrentPath:               "/test",
		GlobalSearchActive:        true,
		GlobalSearchQuery:         "foo",
		GlobalSearchCaseSensitive: true,
		GlobalSearchCursorPos:     3,
		GlobalSearchIndex:         5,
		GlobalSearchResults:       []GlobalSearchResult{},
		GlobalSearchRootPath:      "/test",
		GlobalSearchID:            3,
	}

	reducer := NewStateReducer()

	prevID := state.GlobalSearchID
	if _, err := reducer.Reduce(state, GlobalSearchResetQueryAction{}); err != nil {
		t.Fatalf("Failed to reset global search query: %v", err)
	}

	if !state.GlobalSearchActive {
		t.Error("GlobalSearch should remain active after reset")
	}
	if state.GlobalSearchQuery != "" {
		t.Error("GlobalSearch query should be empty after reset")
	}
	if state.GlobalSearchCaseSensitive {
		t.Error("GlobalSearch should reset case sensitivity when query is cleared")
	}
	if state.GlobalSearchIndex != 0 {
		t.Errorf("GlobalSearch index should reset to 0 after reset, got %d", state.GlobalSearchIndex)
	}
	if state.GlobalSearchID == prevID {
		t.Error("GlobalSearch ID should increment after reset to invalidate pending callbacks")
	}
	if state.GlobalSearchCursorPos != 0 {
		t.Errorf("Cursor position should reset to 0, got %d", state.GlobalSearchCursorPos)
	}
}

func TestGlobalSearchClearAction(t *testing.T) {
	state := &AppState{
		CurrentPath:         "/test",
		GlobalSearchActive:  true,
		GlobalSearchQuery:   "test",
		GlobalSearchIndex:   0,
		GlobalSearchResults: []GlobalSearchResult{},
	}

	reducer := NewStateReducer()

	if _, err := reducer.Reduce(state, GlobalSearchClearAction{}); err != nil {
		t.Fatalf("Failed to clear global search: %v", err)
	}

	if state.GlobalSearchActive {
		t.Error("GlobalSearch should be inactive after clear")
	}
	if state.GlobalSearchQuery != "" {
		t.Error("GlobalSearch query should be empty after clear")
	}
}

func runGlobalSearchNavigationTest(t *testing.T, initialIndex int, direction string, expected []int) {
	t.Helper()
	state := &AppState{
		CurrentPath:        "/test",
		GlobalSearchActive: true,
		GlobalSearchIndex:  initialIndex,
		GlobalSearchResults: []GlobalSearchResult{
			{FilePath: "/test/file1.txt", FileName: "file1.txt"},
			{FilePath: "/test/file2.txt", FileName: "file2.txt"},
			{FilePath: "/test/file3.txt", FileName: "file3.txt"},
			{FilePath: "/test/file4.txt", FileName: "file4.txt"},
		},
	}

	reducer := NewStateReducer()

	for step, want := range expected {
		if _, err := reducer.Reduce(state, GlobalSearchNavigateAction{Direction: direction}); err != nil {
			t.Fatalf("Failed to navigate %s on step %d: %v", direction, step+1, err)
		}

		if state.GlobalSearchIndex != want {
			t.Fatalf("GlobalSearch index should be %d after step %d going %s, got %d", want, step+1, direction, state.GlobalSearchIndex)
		}
	}
}

func TestGlobalSearchNavigateUp(t *testing.T) {
	runGlobalSearchNavigationTest(t, 2, "up", []int{1, 0, 0})
}

func TestGlobalSearchNavigateDown(t *testing.T) {
	runGlobalSearchNavigationTest(t, 1, "down", []int{2, 3, 3})
}

func TestGlobalSearchOpenAction(t *testing.T) {
	// This test would need filesystem mocking, so we'll do a basic check
	state := &AppState{
		CurrentPath:          "/test",
		GlobalSearchActive:   true,
		GlobalSearchIndex:    0,
		GlobalSearchRootPath: "/test",
		GlobalSearchResults: []GlobalSearchResult{
			{
				FilePath:  "/test/subdir/file.txt",
				FileName:  "file.txt",
				DirPath:   "/test/subdir",
				Score:     100,
				FileEntry: FileEntry{Name: "file.txt", IsDir: false},
			},
		},
		Files:         []FileEntry{},
		SelectedIndex: -1,
	}

	// We can't fully test this without mocking filesystem, but we can check that
	// the action is dispatched without error when there are results
	reducer := NewStateReducer()

	// This will fail because subdir doesn't exist, but we're testing the action dispatch
	// We just verify it doesn't panic and handles the error gracefully
	_, _ = reducer.Reduce(state, GlobalSearchOpenAction{})
}

func TestGlobalSearchEmptyResults(t *testing.T) {
	state := &AppState{
		CurrentPath:         "/test",
		GlobalSearchActive:  true,
		GlobalSearchQuery:   "nonexistent",
		GlobalSearchIndex:   0,
		GlobalSearchResults: []GlobalSearchResult{},
	}

	reducer := NewStateReducer()

	// Navigate on empty results (should be no-op)
	if _, err := reducer.Reduce(state, GlobalSearchNavigateAction{Direction: "down"}); err != nil {
		t.Fatalf("Failed to navigate on empty results: %v", err)
	}

	if state.GlobalSearchIndex != 0 {
		t.Errorf("GlobalSearch index should stay at 0 with empty results")
	}

	// Open on empty results should be no-op
	if _, err := reducer.Reduce(state, GlobalSearchOpenAction{}); err != nil {
		t.Fatalf("Failed to open on empty results: %v", err)
	}
}

func TestGlobalSearchAsyncCallback(t *testing.T) {
	// Test that GlobalSearchStartAction initiates async search
	state := &AppState{
		CurrentPath:         "/tmp",
		GlobalSearchActive:  false,
		GlobalSearchQuery:   "",
		GlobalSearchResults: []GlobalSearchResult{},
		ScreenHeight:        24,
		ScreenWidth:         80,
	}

	reducer := NewStateReducer()

	// Start global search - this should trigger async search
	_, err := reducer.Reduce(state, GlobalSearchStartAction{})
	if err != nil {
		t.Errorf("Failed to start global search: %v", err)
	}

	if !state.GlobalSearchActive {
		t.Error("GlobalSearch should be active")
	}

	// Give async search a moment to start (crude but works for test)
	time.Sleep(50 * time.Millisecond)

	// The state should reflect that search is in progress
	// Note: This is a simple test; in production you'd want better async testing
}

func TestClearGlobalSearchStateMethod(t *testing.T) {
	state := &AppState{
		GlobalSearchActive:     true,
		GlobalSearchQuery:      "test",
		GlobalSearchIndex:      5,
		GlobalSearchResults:    []GlobalSearchResult{{FilePath: "/test"}},
		GlobalSearchInProgress: true,
		GlobalSearchRootPath:   "/test",
		GlobalSearchIndexStatus: IndexTelemetry{
			RootPath:     "/tmp",
			FilesIndexed: 42,
			Ready:        true,
		},
	}

	state.clearGlobalSearch(false)

	if state.GlobalSearchActive {
		t.Error("GlobalSearchActive should be false")
	}
	if state.GlobalSearchQuery != "" {
		t.Error("GlobalSearchQuery should be empty")
	}
	if state.GlobalSearchIndex != 0 {
		t.Error("GlobalSearchIndex should be 0")
	}
	if state.GlobalSearchInProgress {
		t.Error("GlobalSearchInProgress should be false")
	}
	if len(state.GlobalSearchResults) != 0 {
		t.Error("GlobalSearchResults should be empty")
	}
	if state.GlobalSearchIndexStatus.RootPath != "" {
		t.Errorf("GlobalSearchIndexStatus should be reset, got %#v", state.GlobalSearchIndexStatus)
	}
}

func TestGlobalSearchOpenActionWithHistory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	state := &AppState{
		CurrentPath:          tmpDir,
		GlobalSearchActive:   true,
		GlobalSearchIndex:    0,
		GlobalSearchRootPath: tmpDir,
		GlobalSearchResults: []GlobalSearchResult{
			{
				FilePath:  filePath,
				FileName:  "file.txt",
				DirPath:   tmpDir,
				Score:     100,
				FileEntry: FileEntry{Name: "file.txt", FullPath: filePath, IsDir: false},
			},
		},
		Files:         []FileEntry{},
		History:       []string{tmpDir},
		HistoryIndex:  0,
		SelectedIndex: -1,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()

	// Execute GlobalSearchOpenAction
	_, _ = reducer.Reduce(state, GlobalSearchOpenAction{})

	// Verify global search was cleared
	if state.GlobalSearchActive {
		t.Error("GlobalSearch should be inactive after open action")
	}

	// Verify we attempted to navigate (either succeeded or failed gracefully)
	// The important part is that clearGlobalSearch was called
	if state.GlobalSearchQuery != "" {
		t.Error("GlobalSearch query should be cleared")
	}
}

func TestClearGlobalSearchRemembersQueryAndRoot(t *testing.T) {
	state := &AppState{
		GlobalSearchActive:   true,
		GlobalSearchQuery:    "foo",
		GlobalSearchRootPath: "/tmp",
		GlobalSearchIndex:    4,
		GlobalSearchScroll:   1,
		GlobalSearchResults: []GlobalSearchResult{
			{FilePath: "/tmp/a", FileName: "a", DirPath: "/tmp"},
			{FilePath: "/tmp/b", FileName: "b", DirPath: "/tmp"},
			{FilePath: "/tmp/c", FileName: "c", DirPath: "/tmp"},
			{FilePath: "/tmp/d", FileName: "d", DirPath: "/tmp"},
			{FilePath: "/tmp/sel", FileName: "sel", DirPath: "/tmp"},
		},
	}

	state.clearGlobalSearch(false)

	if state.LastGlobalSearchQuery != "foo" {
		t.Fatalf("Expected last query to be remembered as 'foo', got '%s'", state.LastGlobalSearchQuery)
	}
	if state.LastGlobalSearchRootPath != "/tmp" {
		t.Fatalf("Expected last root path to be '/tmp', got '%s'", state.LastGlobalSearchRootPath)
	}
	if state.LastGlobalSearchIndex != 4 {
		t.Fatalf("Expected last index to be 4, got %d", state.LastGlobalSearchIndex)
	}
	if state.LastGlobalSearchScroll != 1 {
		t.Fatalf("Expected last scroll to be 1, got %d", state.LastGlobalSearchScroll)
	}
	if state.LastGlobalSearchSelectionPath != "/tmp/sel" {
		t.Fatalf("Expected last selection path to be '/tmp/sel', got '%s'", state.LastGlobalSearchSelectionPath)
	}
}

func TestClearGlobalSearchPreservesMemoryWhenQueryEmpty(t *testing.T) {
	state := &AppState{
		GlobalSearchActive:            true,
		GlobalSearchQuery:             "",
		GlobalSearchRootPath:          "/tmp",
		LastGlobalSearchQuery:         "prev",
		LastGlobalSearchRootPath:      "/tmp",
		LastGlobalSearchIndex:         7,
		LastGlobalSearchScroll:        3,
		LastGlobalSearchSelectionPath: "/tmp/prev",
	}

	state.clearGlobalSearch(false)

	if state.LastGlobalSearchQuery != "prev" {
		t.Fatalf("Expected last query memory to remain 'prev', got '%s'", state.LastGlobalSearchQuery)
	}
	if state.LastGlobalSearchRootPath != "/tmp" {
		t.Fatalf("Expected last root memory to remain '/tmp', got '%s'", state.LastGlobalSearchRootPath)
	}
	if state.LastGlobalSearchIndex != 7 {
		t.Fatalf("Expected last index memory to remain 7, got %d", state.LastGlobalSearchIndex)
	}
	if state.LastGlobalSearchScroll != 3 {
		t.Fatalf("Expected last scroll memory to remain 3, got %d", state.LastGlobalSearchScroll)
	}
	if state.LastGlobalSearchSelectionPath != "/tmp/prev" {
		t.Fatalf("Expected last selection memory to remain '/tmp/prev', got '%s'", state.LastGlobalSearchSelectionPath)
	}
}

func TestClearGlobalSearchForgetMemory(t *testing.T) {
	state := &AppState{
		GlobalSearchActive:            true,
		GlobalSearchQuery:             "",
		GlobalSearchRootPath:          "/tmp",
		LastGlobalSearchQuery:         "prev",
		LastGlobalSearchRootPath:      "/tmp",
		LastGlobalSearchIndex:         7,
		LastGlobalSearchScroll:        3,
		LastGlobalSearchSelectionPath: "/tmp/prev",
	}

	state.clearGlobalSearch(true)

	if state.LastGlobalSearchQuery != "" {
		t.Fatalf("Expected last query memory to be cleared, got '%s'", state.LastGlobalSearchQuery)
	}
	if state.LastGlobalSearchRootPath != "" {
		t.Fatalf("Expected last root memory to be cleared, got '%s'", state.LastGlobalSearchRootPath)
	}
	if state.LastGlobalSearchIndex != 0 {
		t.Fatalf("Expected last index memory to be cleared, got %d", state.LastGlobalSearchIndex)
	}
	if state.LastGlobalSearchScroll != 0 {
		t.Fatalf("Expected last scroll memory to be cleared, got %d", state.LastGlobalSearchScroll)
	}
	if state.LastGlobalSearchSelectionPath != "" {
		t.Fatalf("Expected last selection memory to be cleared, got '%s'", state.LastGlobalSearchSelectionPath)
	}
}

func TestGlobalSearchFullPathMatching(t *testing.T) {
	// Test that global search matches on full path, not just filename
	// Create a temporary directory structure
	tempDir := t.TempDir()

	// Create directory structure: src/components/ and internal/models/
	componentsDir := filepath.Join(tempDir, "src", "components")
	modelsDir := filepath.Join(tempDir, "internal", "models")

	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		t.Fatalf("Failed to create components dir: %v", err)
	}
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("Failed to create models dir: %v", err)
	}

	// Create test files
	if err := os.WriteFile(filepath.Join(componentsDir, "button.go"), []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write button.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(componentsDir, "modal.go"), []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write modal.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "user.go"), []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write user.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	// Test search for "srcbtn" - should find src/components/button.go
	searcher := searchpkg.NewGlobalSearcher(tempDir, false, nil)
	results := searcher.SearchRecursive("srcbtn", false)

	found := false
	for _, r := range results {
		relPath, _ := filepath.Rel(tempDir, r.FilePath)
		relPath = filepath.ToSlash(relPath)
		if relPath == "src/components/button.go" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find 'src/components/button.go' with query 'srcbtn', but got: %v", results)
	}

	// Test search for "imod" - should find internal/models files
	results = searcher.SearchRecursive("imod", false)

	if len(results) < 1 {
		t.Errorf("Expected to find files matching 'imod' in internal/models, but got: %v", results)
	}

	for _, r := range results {
		relPath, _ := filepath.Rel(tempDir, r.FilePath)
		relPath = filepath.ToSlash(relPath)
		if !strings.HasPrefix(relPath, "internal/models/") {
			t.Errorf("Expected result to start with 'internal/models/', but got: %s", relPath)
		}
	}

	// Test search for "comp" - should find src/components files
	results = searcher.SearchRecursive("comp", false)

	found = false
	for _, r := range results {
		relPath, _ := filepath.Rel(tempDir, r.FilePath)
		relPath = filepath.ToSlash(relPath)
		if strings.HasPrefix(relPath, "src/components/") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find files in 'src/components/' with query 'comp', but got: %v", results)
	}
}
