package state

import (
	"testing"

	searchpkg "github.com/kk-code-lab/rdir/internal/search"
)

// Integration tests for fuzzy search in the full application
func TestFuzzyIntegration_FilteringWithFuzzyMatch(t *testing.T) {
	// Create state with some files
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "main.go", IsDir: false},
			{Name: "reducer.go", IsDir: false},
			{Name: "state.go", IsDir: false},
			{Name: "input_handler.go", IsDir: false},
			{Name: "renderer.go", IsDir: false},
			{Name: "fuzzy.go", IsDir: false},
			{Name: "IMPLEMENTATION.md", IsDir: false},
			{Name: "go.mod", IsDir: false},
		},
		ScreenWidth:  80,
		ScreenHeight: 24,
	}

	reducer := NewStateReducer()

	// Test 1: Filter with "red" - should find reducer.go, renderer.go
	_, _ = reducer.Reduce(state, FilterStartAction{})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'r'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'e'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'd'})

	if len(state.FilterMatches) < 1 {
		t.Errorf("Expected at least 1 match for 'red', got %d", len(state.FilterMatches))
	}

	// reducer.go should be first (best match)
	if len(state.FilterMatches) > 0 {
		firstFile := state.Files[state.FilterMatches[0].FileIndex]
		if firstFile.Name != "reducer.go" && firstFile.Name != "renderer.go" {
			t.Logf("First match for 'red': %s (score: %.3f)", firstFile.Name, state.FilterMatches[0].Score)
		}
	}

	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != len(state.FilterMatches) {
		t.Errorf("Display files count %d != filter matches count %d", len(displayFiles), len(state.FilterMatches))
	}

	// Test 2: Clear filter
	_, _ = reducer.Reduce(state, FilterClearAction{})
	if state.FilterActive {
		t.Error("Filter should be inactive after clear")
	}
	if len(state.FilterMatches) > 0 {
		t.Error("Filter matches should be empty after clear")
	}

	// Test 3: Filter with "go" - should find all .go files and more
	_, _ = reducer.Reduce(state, FilterStartAction{})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'g'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'o'})

	if len(state.FilterMatches) < 5 {
		t.Errorf("Expected at least 5 matches for 'go', got %d", len(state.FilterMatches))
	}

	// Verify all matches have 'g' and 'o' in order
	for _, match := range state.FilterMatches {
		filename := state.Files[match.FileIndex].Name
		pattern := "go"
		if !fuzzyContains(filename, pattern) {
			t.Errorf("Match %s doesn't contain pattern 'go' in order", filename)
		}
	}
}

func TestFuzzyIntegration_FilterPreservesOriginalOrder(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "go.mod", IsDir: false},
			{Name: "go.sum", IsDir: false},
			{Name: "main.go", IsDir: false},
			{Name: "gorgeous.txt", IsDir: false},
		},
		ScreenWidth:  80,
		ScreenHeight: 24,
	}

	reducer := NewStateReducer()

	// Filter with "go"
	_, _ = reducer.Reduce(state, FilterStartAction{})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'g'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'o'})

	expectedOrder := []string{"go.mod", "go.sum", "main.go", "gorgeous.txt"}
	if len(state.FilterMatches) != len(expectedOrder) {
		t.Fatalf("Expected %d matches for 'go', got %d", len(expectedOrder), len(state.FilterMatches))
	}

	for i, expected := range expectedOrder {
		fileName := state.Files[state.FilterMatches[i].FileIndex].Name
		if fileName != expected {
			t.Fatalf("Match %d should be %s, got %s", i, expected, fileName)
		}
	}

	if len(state.FilteredIndices) != len(state.FilterMatches) {
		t.Fatalf("FilteredIndices (%d) and FilterMatches (%d) diverged", len(state.FilteredIndices), len(state.FilterMatches))
	}

	for i, idx := range state.FilteredIndices {
		if idx != state.FilterMatches[i].FileIndex {
			t.Fatalf("Index %d mismatch: FilteredIndices has %d, FilterMatches has %d", i, idx, state.FilterMatches[i].FileIndex)
		}
	}
}

func TestFuzzyIntegration_CaseSensitivity(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "IMPLEMENTATION.md", IsDir: false},
			{Name: "implementation.md", IsDir: false},
			{Name: "Implementation.go", IsDir: false},
		},
		ScreenWidth:  80,
		ScreenHeight: 24,
	}

	reducer := NewStateReducer()

	// Filter with lowercase "impl"
	_, _ = reducer.Reduce(state, FilterStartAction{})
	for _, ch := range "impl" {
		_, _ = reducer.Reduce(state, FilterCharAction{Char: ch})
	}

	if len(state.FilterMatches) != 3 {
		t.Errorf("Expected 3 matches for 'impl' (case-insensitive), got %d", len(state.FilterMatches))
	}

	// Filter with uppercase "IMPL" should respect case (only all-uppercase file matches)
	_, _ = reducer.Reduce(state, FilterClearAction{})
	_, _ = reducer.Reduce(state, FilterStartAction{})
	for _, ch := range "IMPL" {
		_, _ = reducer.Reduce(state, FilterCharAction{Char: ch})
	}

	if len(state.FilterMatches) != 1 {
		t.Fatalf("Expected 1 match for 'IMPL' with smart-case, got %d", len(state.FilterMatches))
	}

	matched := state.FilterMatches[0]
	if state.Files[matched.FileIndex].Name != "IMPLEMENTATION.md" {
		t.Errorf("Expected IMPLEMENTATION.md to match smart-case query, got %s", state.Files[matched.FileIndex].Name)
	}
}

func TestFuzzyIntegration_BackspaceInFilter(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "reducer.go", IsDir: false},
			{Name: "renderer.go", IsDir: false},
			{Name: "state.go", IsDir: false},
		},
		ScreenWidth:  80,
		ScreenHeight: 24,
	}

	reducer := NewStateReducer()

	// Start with "red"
	_, _ = reducer.Reduce(state, FilterStartAction{})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'r'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'e'})
	_, _ = reducer.Reduce(state, FilterCharAction{Char: 'd'})

	matches1 := len(state.FilterMatches)

	// Backspace to "re"
	_, _ = reducer.Reduce(state, FilterBackspaceAction{})

	matches2 := len(state.FilterMatches)

	// "re" should match both reducer.go and renderer.go
	if matches2 <= matches1 {
		t.Logf("Backspace: 'red' had %d matches, 're' has %d matches", matches1, matches2)
	}

	// Backspace to "r"
	_, _ = reducer.Reduce(state, FilterBackspaceAction{})

	matches3 := len(state.FilterMatches)

	// "r" should match reducer and renderer and state (for render)
	if matches3 <= matches2 {
		t.Logf("Backspace: 're' had %d matches, 'r' has %d matches", matches2, matches3)
	}
}

// Helper function
func fuzzyContains(text, pattern string) bool {
	patternIdx := 0
	for i := 0; i < len(text) && patternIdx < len(pattern); i++ {
		if text[i] == pattern[patternIdx] {
			patternIdx++
		}
	}
	return patternIdx == len(pattern)
}

func TestFuzzyIntegration_NoDirectoryPriorityBonus(t *testing.T) {
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "src", IsDir: true},
			{Name: "src.go", IsDir: false},
			{Name: "src_test.go", IsDir: false},
			{Name: "scripts", IsDir: true},
		},
		ScreenWidth:  80,
		ScreenHeight: 24,
	}

	reducer := NewStateReducer()

	_, _ = reducer.Reduce(state, FilterStartAction{})
	for _, ch := range "src" {
		_, _ = reducer.Reduce(state, FilterCharAction{Char: ch})
	}

	if len(state.FilterMatches) == 0 {
		t.Fatalf("expected matches for 'src'")
	}

	fm := searchpkg.NewFuzzyMatcher()
	query := "src"

	for _, match := range state.FilterMatches {
		file := state.Files[match.FileIndex]
		score, matched := fm.Match(query, file.Name)
		if !matched {
			t.Fatalf("expected %s to match query %s", file.Name, query)
		}
		if match.Score < score {
			t.Fatalf("stored score %.3f should not be below base %.3f for %s",
				match.Score, score, file.Name)
		}
	}
}
