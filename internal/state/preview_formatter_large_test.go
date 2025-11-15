package state

import "testing"

func TestJSONFormatterSkipsLargeFiles(t *testing.T) {
	size := formattedPreviewMaxBytes + 1024
	content := make([]byte, size)
	for i := range content {
		content[i] = ' '
	}
	copy(content, []byte(`{"big": true}`))
	info := fakeFileInfo{name: "big.json", size: int64(size)}

	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: content[:formattedPreviewMaxBytes], // simulate head read
	}
	preview := &PreviewData{}
	jsonPreviewFormatter{}.Format(ctx, preview)

	if len(preview.FormattedTextLines) != 0 {
		t.Fatalf("expected no formatted lines for large file")
	}
	if preview.FormattedUnavailableReason == "" {
		t.Fatalf("expected formatted unavailable reason for large file")
	}
}
