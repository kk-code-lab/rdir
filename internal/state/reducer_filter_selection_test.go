package state

import "testing"

func TestFilterTypingKeepsSelectionWhenMatchRemains(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha.txt"},
			{Name: "beta.txt"},
			{Name: "betamax.txt"},
			{Name: "gamma.txt"},
		},
		ScreenHeight: 40,
		ScreenWidth:  120,
	}

	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'b'}); err != nil {
		t.Fatalf("type b: %v", err)
	}
	// Move to the second match (betamax.txt)
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("navigate down: %v", err)
	}

	selectedBefore := state.SelectedIndex
	current := state.getCurrentFile()
	if current == nil || current.Name != "betamax.txt" {
		t.Fatalf("expected to be on betamax.txt before refining, got %v", current)
	}

	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'e'}); err != nil {
		t.Fatalf("type e: %v", err)
	}

	if state.SelectedIndex != selectedBefore {
		t.Fatalf("selection should stay on betamax.txt (index %d), got %d", selectedBefore, state.SelectedIndex)
	}
}

func TestFilterTypingKeepsRelativePositionWhenMatchDrops(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "bench.go"},
			{Name: "bend.go"},
			{Name: "beta.go"},
		},
		ScreenHeight: 40,
		ScreenWidth:  120,
	}

	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'b'}); err != nil {
		t.Fatalf("type b: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'e'}); err != nil {
		t.Fatalf("type e: %v", err)
	}

	// Move to display index 2 (third visible entry)
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("navigate down: %v", err)
	}
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("navigate down: %v", err)
	}

	if idx := state.getDisplaySelectedIndex(); idx != 2 {
		t.Fatalf("expected to be on display index 2 before refining, got %d", idx)
	}

	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'n'}); err != nil {
		t.Fatalf("type n: %v", err)
	}

	displayIdx := state.getDisplaySelectedIndex()
	if displayIdx != 1 {
		t.Fatalf("selection should clamp near old position (expected 1), got %d", displayIdx)
	}

	if displayCount := len(state.getDisplayFiles()); displayCount != 2 {
		t.Fatalf("expected exactly 2 matches after typing 'n', got %d", displayCount)
	}
}

func TestFilterBackspaceKeepsSelectionWhenClearingQuery(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		CurrentPath: "/test",
		Files: []FileEntry{
			{Name: "alpha.txt"},
			{Name: "beta.txt"},
			{Name: "betamax.txt"},
			{Name: "gamma.txt"},
		},
		ScreenHeight: 40,
		ScreenWidth:  120,
	}

	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'b'}); err != nil {
		t.Fatalf("type b: %v", err)
	}
	if _, err := reducer.Reduce(state, NavigateDownAction{}); err != nil {
		t.Fatalf("navigate down: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'e'}); err != nil {
		t.Fatalf("type e: %v", err)
	}

	current := state.getCurrentFile()
	if current == nil || current.Name != "betamax.txt" {
		t.Fatalf("expected to be on betamax.txt before backspace, got %v", current)
	}

	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("backspace once: %v", err)
	}
	if after := state.getCurrentFile(); after == nil || after.Name != "betamax.txt" {
		t.Fatalf("selection should stay on betamax.txt after first backspace, got %v", after)
	}

	if _, err := reducer.Reduce(state, FilterBackspaceAction{}); err != nil {
		t.Fatalf("backspace twice: %v", err)
	}

	if state.FilterQuery != "" {
		t.Fatalf("expected empty query after clearing, got %q", state.FilterQuery)
	}

	if after := state.getCurrentFile(); after == nil || after.Name != "betamax.txt" {
		t.Fatalf("selection should stay on betamax.txt after clearing query, got %v", after)
	}
}
