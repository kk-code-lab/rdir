package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewEnterAndExitFullScreen(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		PreviewData:  &PreviewData{TextLines: []string{"line"}},
		ScreenHeight: 10,
	}

	if _, err := reducer.Reduce(state, PreviewEnterFullScreenAction{}); err != nil {
		t.Fatalf("enter fullscreen failed: %v", err)
	}
	if !state.PreviewFullScreen {
		t.Fatal("expected preview fullscreen flag to be set")
	}

	if _, err := reducer.Reduce(state, PreviewExitFullScreenAction{}); err != nil {
		t.Fatalf("exit fullscreen failed: %v", err)
	}
	if state.PreviewFullScreen {
		t.Fatal("expected preview fullscreen flag to be cleared")
	}
}

func TestPreviewScrollRequiresFullScreen(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		PreviewData:  &PreviewData{TextLines: make([]string, 20)},
		ScreenHeight: 8,
	}

	if _, err := reducer.Reduce(state, PreviewScrollDownAction{}); err != nil {
		t.Fatalf("scroll action failed: %v", err)
	}
	if state.PreviewScrollOffset != 0 {
		t.Fatalf("scroll should be ignored outside fullscreen, got %d", state.PreviewScrollOffset)
	}

	state.PreviewFullScreen = true
	if _, err := reducer.Reduce(state, PreviewScrollDownAction{}); err != nil {
		t.Fatalf("scroll down in fullscreen failed: %v", err)
	}
	if state.PreviewScrollOffset != 1 {
		t.Fatalf("expected offset 1, got %d", state.PreviewScrollOffset)
	}
}

func TestPreviewScrollPagingClampsOffsets(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		PreviewData:       &PreviewData{TextLines: make([]string, 100)},
		ScreenHeight:      8, // visible lines = 6
		PreviewFullScreen: true,
	}

	if _, err := reducer.Reduce(state, PreviewScrollPageDownAction{}); err != nil {
		t.Fatalf("page down failed: %v", err)
	}
	if state.PreviewScrollOffset != 6 {
		t.Fatalf("expected offset 6 after page down, got %d", state.PreviewScrollOffset)
	}

	if _, err := reducer.Reduce(state, PreviewScrollToEndAction{}); err != nil {
		t.Fatalf("scroll to end failed: %v", err)
	}
	max := state.maxPreviewScrollOffset()
	if state.PreviewScrollOffset != max {
		t.Fatalf("expected offset %d, got %d", max, state.PreviewScrollOffset)
	}

	if _, err := reducer.Reduce(state, PreviewScrollToStartAction{}); err != nil {
		t.Fatalf("scroll to start failed: %v", err)
	}
	if state.PreviewScrollOffset != 0 {
		t.Fatalf("expected offset reset to 0, got %d", state.PreviewScrollOffset)
	}
}

func TestGeneratePreviewPreservesOffsetForSameFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-preview-preserve-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\nline6"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	reducer := NewStateReducer()
	state := &AppState{
		ScreenHeight: 4,
		ScreenWidth:  80,
	}

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("change directory failed: %v", err)
	}

	if err := reducer.generatePreview(state); err != nil {
		t.Fatalf("initial preview failed: %v", err)
	}

	state.PreviewFullScreen = true
	state.PreviewScrollOffset = 2

	if err := reducer.generatePreview(state); err != nil {
		t.Fatalf("regenerating preview failed: %v", err)
	}

	if state.PreviewScrollOffset != 2 {
		t.Fatalf("offset should remain 2 for same file, got %d", state.PreviewScrollOffset)
	}
	if !state.PreviewFullScreen {
		t.Fatal("fullscreen should remain when preview target unchanged")
	}
}

func TestGeneratePreviewResetsOffsetForNewFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-preview-reset-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	files := []string{"a.txt", "b.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("content\nmore\nlines"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	reducer := NewStateReducer()
	state := &AppState{
		ScreenHeight: 10,
		ScreenWidth:  80,
	}

	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("change directory failed: %v", err)
	}

	state.PreviewData = &PreviewData{Name: "a.txt", IsDir: false}
	state.PreviewFullScreen = true
	state.PreviewScrollOffset = 3
	state.SelectedIndex = 1 // select b.txt

	if err := reducer.generatePreview(state); err != nil {
		t.Fatalf("generate preview failed: %v", err)
	}

	if state.PreviewScrollOffset != 0 {
		t.Fatalf("offset should reset on new file, got %d", state.PreviewScrollOffset)
	}
	if state.PreviewFullScreen {
		t.Fatal("fullscreen should drop when preview target changes")
	}
}

func TestTogglePreviewWrapRequiresFullscreen(t *testing.T) {
	reducer := NewStateReducer()
	state := &AppState{
		PreviewData: &PreviewData{TextLines: []string{"line"}},
	}

	if _, err := reducer.Reduce(state, TogglePreviewWrapAction{}); err != nil {
		t.Fatalf("toggle wrap failed: %v", err)
	}
	if state.PreviewWrap {
		t.Fatalf("wrap should not toggle when not fullscreen")
	}

	state.PreviewFullScreen = true
	if _, err := reducer.Reduce(state, TogglePreviewWrapAction{}); err != nil {
		t.Fatalf("toggle wrap fullscreen failed: %v", err)
	}
	if !state.PreviewWrap {
		t.Fatalf("wrap should toggle when fullscreen")
	}
}
