package state

import "testing"

func TestMarkdownPreviewFormatterFormatsContent(t *testing.T) {
	content := "# Title\n\nSome *emph* and **bold** text with [link](http://example.com)\n"
	info := fakeFileInfo{name: "doc.md", size: int64(len(content))}
	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: []byte(content),
	}
	preview := &PreviewData{}
	markdownPreviewFormatter{}.Format(ctx, preview)

	if len(preview.FormattedTextLines) == 0 {
		t.Fatalf("expected formatted markdown lines")
	}
	if preview.FormattedTextLines[0] != "H1 Title" {
		t.Fatalf("expected heading to be normalized, got %q", preview.FormattedTextLines[0])
	}
	if preview.FormattedUnavailableReason != "" {
		t.Fatalf("unexpected unavailable reason: %s", preview.FormattedUnavailableReason)
	}
	if len(preview.TextLines) == 0 {
		t.Fatalf("expected raw text lines to remain")
	}
}

func TestMarkdownPreviewFormatterRespectsSizeLimit(t *testing.T) {
	size := formattedPreviewMaxBytes + 2048
	content := make([]byte, formattedPreviewMaxBytes)
	for i := range content {
		content[i] = '#'
	}
	info := fakeFileInfo{name: "large.md", size: int64(size)}
	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: content,
	}
	preview := &PreviewData{}
	markdownPreviewFormatter{}.Format(ctx, preview)

	if len(preview.FormattedTextLines) != 0 {
		t.Fatalf("expected formatted lines to be skipped for large file")
	}
	if preview.FormattedUnavailableReason == "" {
		t.Fatalf("expected unavailable reason for large markdown file")
	}
}
