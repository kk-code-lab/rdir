package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshDirectoryKeepsSelection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	files := []string{"alpha.txt", "beta.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}
	reducer := NewStateReducer()

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load directory: %v", err)
	}

	betaIdx := findFileIndexByName(state.Files, "beta.txt")
	if betaIdx == -1 {
		t.Fatalf("beta.txt not found in initial listing")
	}

	state.SelectedIndex = betaIdx
	state.updateScrollVisibility()

	// Add another file to prove refresh re-reads directory but keeps previous selection
	if err := os.WriteFile(filepath.Join(tmpDir, "gamma.txt"), []byte("gamma"), 0o644); err != nil {
		t.Fatalf("failed to create gamma.txt: %v", err)
	}

	if _, err := reducer.Reduce(state, RefreshDirectoryAction{}); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	current := state.CurrentFile()
	if current == nil || current.Name != "beta.txt" {
		t.Fatalf("expected selection to stay on beta.txt, got %v", current)
	}
}

func TestRefreshDirectoryKeepsFilterState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}
	reducer := NewStateReducer()

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load directory: %v", err)
	}

	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("failed to start filter: %v", err)
	}

	for _, ch := range []rune{'a', 'l', 'p', 'h'} {
		if _, err := reducer.Reduce(state, FilterCharAction{Char: ch}); err != nil {
			t.Fatalf("failed to type filter char: %v", err)
		}
	}

	current := state.CurrentFile()
	if current == nil || current.Name != "alpha.txt" {
		t.Fatalf("expected alpha.txt selected before refresh, got %v", current)
	}

	// Touch files to ensure their mod times differ after refresh
	if err := os.WriteFile(filepath.Join(tmpDir, "delta.txt"), []byte("delta"), 0o644); err != nil {
		t.Fatalf("failed to create delta.txt: %v", err)
	}

	if _, err := reducer.Reduce(state, RefreshDirectoryAction{}); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if !state.FilterActive {
		t.Fatalf("filter should remain active after refresh")
	}
	if state.FilterQuery != "alph" {
		t.Fatalf("expected filter query 'alph', got %q", state.FilterQuery)
	}

	current = state.CurrentFile()
	if current == nil || current.Name != "alpha.txt" {
		t.Fatalf("expected selection to remain on alpha.txt, got %v", current)
	}
}

func TestRefreshDirectoryHandlesMissingSelection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}
	reducer := NewStateReducer()

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("failed to load directory: %v", err)
	}

	betaIdx := findFileIndexByName(state.Files, "beta.txt")
	if betaIdx == -1 {
		t.Fatalf("beta.txt not found in initial listing")
	}
	state.SelectedIndex = betaIdx
	state.updateScrollVisibility()

	if err := os.Remove(filepath.Join(tmpDir, "beta.txt")); err != nil {
		t.Fatalf("failed to remove beta.txt: %v", err)
	}

	if _, err := reducer.Reduce(state, RefreshDirectoryAction{}); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	current := state.CurrentFile()
	if current == nil {
		t.Fatal("expected a selection after refresh")
	}
	if current.Name != "alpha.txt" {
		t.Fatalf("expected selection to fall back to alpha.txt, got %s", current.Name)
	}
}
