package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONPreviewFormatterFormatsContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-json-preview-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "data.json")
	content := `{"name":"rdir","tags":["cli","tui"],"meta":{"size":42}}`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	ctx := previewFormatContext{
		path:    filePath,
		info:    info,
		content: []byte(content),
	}
	preview := &PreviewData{}
	jsonPreviewFormatter{}.Format(ctx, preview)

	if len(preview.FormattedTextLines) == 0 {
		t.Fatalf("expected formatted text lines")
	}
	if got := preview.FormattedTextLines[0]; got != "{" {
		t.Fatalf("expected first formatted line '{', got %q", got)
	}
	if len(preview.TextLines) == 0 {
		t.Fatalf("expected raw text lines to remain available")
	}
	if preview.TextTruncated {
		t.Fatalf("json formatter should mark preview as not truncated")
	}
	if preview.TextBytesRead != int64(len(content)) {
		t.Fatalf("TextBytesRead mismatch, want %d got %d", len(content), preview.TextBytesRead)
	}
	if preview.LineCount != len(preview.TextLines) {
		t.Fatalf("LineCount mismatch, want %d got %d", len(preview.TextLines), preview.LineCount)
	}
	if len(preview.FormattedTextLineMeta) != len(preview.FormattedTextLines) {
		t.Fatalf("expected formatted metadata for each line")
	}
	if preview.FormattedUnavailableReason != "" {
		t.Fatalf("unexpected formatted unavailable reason: %s", preview.FormattedUnavailableReason)
	}
}

func TestJSONPreviewFormatterFallsBackWhenTruncated(t *testing.T) {
	content := []byte(`{"partial": true, "value": 1`)
	info := fakeFileInfo{name: "partial.json", size: int64(len(content) + 10)}
	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: content,
	}
	preview := &PreviewData{}
	jsonPreviewFormatter{}.Format(ctx, preview)

	if len(preview.FormattedTextLines) != 0 {
		t.Fatalf("expected no formatted lines for truncated content")
	}
	if len(preview.TextLines) != 0 {
		t.Fatalf("expected no complete raw lines due to truncation, got %d", len(preview.TextLines))
	}
	if len(preview.TextRemainder) == 0 {
		t.Fatalf("expected remainder data for truncated content")
	}
	if preview.TextTruncated != true {
		t.Fatalf("fallback should preserve truncated flag from text formatter")
	}
	if preview.FormattedUnavailableReason == "" {
		t.Fatalf("expected formatted unavailable reason to be set for truncated content")
	}
}

type fakeFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }
