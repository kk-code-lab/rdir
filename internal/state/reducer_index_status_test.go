package state

import (
	"testing"

	searchpkg "github.com/kk-code-lab/rdir/internal/search"
)

func TestReducerUpdatesIndexStatusOnProgress(t *testing.T) {
	reducer := NewStateReducer()
	progress := IndexTelemetry{
		RootPath:     "/tmp/project",
		Building:     true,
		FilesIndexed: 1234,
	}
	state := &AppState{
		GlobalSearchRootPath: "/tmp/project",
		GlobalSearcher:       searchpkg.NewGlobalSearcher("/tmp/project", false, nil),
	}

	newState, err := reducer.Reduce(state, GlobalSearchIndexProgressAction{Progress: progress})
	if err != nil {
		t.Fatalf("reduce returned error: %v", err)
	}
	if !newState.GlobalSearchIndexStatus.Building {
		t.Fatalf("expected Building=true, got %#v", newState.GlobalSearchIndexStatus)
	}
	if newState.GlobalSearchIndexStatus.FilesIndexed != progress.FilesIndexed {
		t.Fatalf("expected FilesIndexed=%d, got %d", progress.FilesIndexed, newState.GlobalSearchIndexStatus.FilesIndexed)
	}
}

func TestReducerIgnoresProgressForDifferentRoot(t *testing.T) {
	reducer := NewStateReducer()
	initial := IndexTelemetry{RootPath: "/tmp/project", Ready: true, FilesIndexed: 5000}
	state := &AppState{
		GlobalSearchRootPath:    "/tmp/project",
		GlobalSearchIndexStatus: initial,
		GlobalSearcher:          searchpkg.NewGlobalSearcher("/tmp/project", false, nil),
	}

	progress := IndexTelemetry{RootPath: "/tmp/other", Ready: true, FilesIndexed: 100}
	newState, err := reducer.Reduce(state, GlobalSearchIndexProgressAction{Progress: progress})
	if err != nil {
		t.Fatalf("reduce returned error: %v", err)
	}
	if newState.GlobalSearchIndexStatus != initial {
		t.Fatalf("expected index status to remain unchanged, got %#v", newState.GlobalSearchIndexStatus)
	}
}
