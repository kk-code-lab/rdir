package state

import (
	"os"
	"path/filepath"
	"testing"
)

type stubPreviewLoader struct {
	lastReq PreviewLoadRequest
	cancel  int
}

func (l *stubPreviewLoader) Start(req PreviewLoadRequest) {
	l.lastReq = req
}

func (l *stubPreviewLoader) Cancel(token int) {
	l.cancel = token
}

// Test that rapid selection changes do not update preview until the debounce fires,
// and that only the latest selection is loaded.
func TestGeneratePreviewDebounceAppliesLatestSelection(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fileA, []byte("content a"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("content b"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	entries, err := readDirectoryEntries(dir)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}

	state := &AppState{
		CurrentPath:     dir,
		ScreenHeight:    40,
		ScreenWidth:     80,
		HideHiddenFiles: true,
	}
	applyDirectoryEntries(state, dir, entries)

	var dispatched []Action
	state.SetDispatch(func(a Action) { dispatched = append(dispatched, a) })
	loader := &stubPreviewLoader{}
	state.PreviewLoader = loader

	reducer := NewStateReducer()

	// First selection (a.txt) schedules a pending load but should not update preview yet.
	if err := reducer.generatePreview(state); err != nil {
		t.Fatalf("generate preview a: %v", err)
	}
	if state.PreviewData != nil {
		t.Fatalf("expected no preview before debounce for first selection")
	}
	firstToken, firstPath, _ := state.previewPendingLoad()
	if firstToken == 0 || firstPath != fileA {
		t.Fatalf("expected pending load for %s, got token=%d path=%s", fileA, firstToken, firstPath)
	}

	// Switch selection quickly to b.txt before debounce fires.
	state.SelectedIndex = 1
	if err := reducer.generatePreview(state); err != nil {
		t.Fatalf("generate preview b: %v", err)
	}
	if state.PreviewData != nil {
		t.Fatalf("expected no preview before debounce for second selection")
	}
	secondToken, secondPath, _ := state.previewPendingLoad()
	if secondToken == 0 || secondPath != fileB {
		t.Fatalf("expected pending load for %s, got token=%d path=%s", fileB, secondToken, secondPath)
	}
	if loader.lastReq.Token != 0 {
		t.Fatalf("loader should not start until debounce fires")
	}

	// Fire debounce manually.
	if _, err := reducer.Reduce(state, PreviewLoadStartAction{Token: secondToken}); err != nil {
		t.Fatalf("start action: %v", err)
	}
	if loader.lastReq.Token != secondToken {
		t.Fatalf("loader Start not invoked with second token (got %d)", loader.lastReq.Token)
	}
	if state.ActivePreviewLoadToken() != secondToken || !state.PreviewLoading {
		t.Fatalf("preview should be marked loading for second token")
	}
	if state.PreviewData != nil {
		t.Fatalf("preview should still be empty until result arrives")
	}

	// Complete load with the second file only.
	data, info, err := buildPreviewData(loader.lastReq.Path, loader.lastReq.HideHidden)
	if err != nil {
		t.Fatalf("build preview: %v", err)
	}
	if _, err := reducer.Reduce(state, PreviewLoadResultAction{
		Token:   loader.lastReq.Token,
		Path:    loader.lastReq.Path,
		Preview: data,
		Info:    info,
	}); err != nil {
		t.Fatalf("result action: %v", err)
	}

	if state.PreviewData == nil || state.PreviewData.Name != "b.txt" {
		t.Fatalf("expected preview of b.txt, got %+v", state.PreviewData)
	}
}
