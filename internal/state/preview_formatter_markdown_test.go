package state

import (
	"fmt"
	"strings"
	"testing"
)

func TestMarkdownPreviewFormatterFormatsContent(t *testing.T) {
	content := "# Title\n\nSome *emph* and **bold** text with [link](http://example.com) and `code`\n- first\n- second\n"
	info := fakeFileInfo{name: "doc.md", size: int64(len(content))}
	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: []byte(content),
	}
	preview := &PreviewData{}
	markdownPreviewFormatter{}.Format(ctx, preview)

	want := []string{
		"# Title",
		"",
		"Some emph and bold text with link (http://example.com) and code",
		"",
		"• first",
		"• second",
	}
	if diff := diffLines(want, preview.FormattedTextLines); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
	if preview.FormattedUnavailableReason != "" {
		t.Fatalf("unexpected unavailable reason: %s", preview.FormattedUnavailableReason)
	}
	if len(preview.FormattedTextLineMeta) != len(preview.FormattedTextLines) {
		t.Fatalf("expected formatted metadata for each line")
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

func TestMarkdownInlineParsingEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "nested emphasis collapses markers",
			line: "Nested **strong and *em* mix**.",
			want: "Nested strong and em mix.",
		},
		{
			name: "backticks protect markers",
			line: "Keep `*stars*` inside code and \\*escaped\\* markers outside.",
			want: "Keep *stars* inside code and *escaped* markers outside.",
		},
		{
			name: "links and images render destinations",
			line: "Image: ![alt](img.png) and [link](https://example.com).",
			want: "Image: alt (img.png) and link (https://example.com).",
		},
		{
			name: "strikethrough renders text",
			line: "Old ~~removed~~ text.",
			want: "Old removed text.",
		},
		{
			name: "autolinks",
			line: "See <https://example.com> or <mailto:test@example.com> now.",
			want: "See https://example.com (https://example.com) or test@example.com (mailto:test@example.com) now.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMarkdownLines([]string{tt.line})
			if len(got) != 1 {
				t.Fatalf("expected single output line, got %d: %#v", len(got), got)
			}
			if got[0] != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got[0])
			}
		})
	}
}

func TestMarkdownBlockParsing(t *testing.T) {
	lines := []string{
		"```go",
		"fmt.Println(\"hello\")",
		"```",
		"",
		"> quoted *text*",
		">",
		"> second paragraph",
		"",
		"1. first",
		"2. second",
		"",
		"Setext heading",
		"-------",
		"",
		"| A | B |",
		"| :--- | ---: |",
		"| c1 | c2 |",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"    [go]",
		"    fmt.Println(\"hello\")",
		"",
		"│ quoted text",
		"│ ",
		"│ second paragraph",
		"",
		"1. first",
		"2. second",
		"",
		"## Setext heading",
		"",
		"┌────┬────┐",
		"│ A  │  B │",
		"├────┼────┤",
		"│ c1 │ c2 │",
		"└────┴────┘",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func diffLines(want, got []string) string {
	if len(want) == len(got) {
		match := true
		for i := range want {
			if want[i] != got[i] {
				match = false
				break
			}
		}
		if match {
			return ""
		}
	}
	var b strings.Builder
	b.WriteString("want:\n")
	for i, line := range want {
		fmt.Fprintf(&b, "%d: %q\n", i, line)
	}
	b.WriteString("got:\n")
	for i, line := range got {
		fmt.Fprintf(&b, "%d: %q\n", i, line)
	}
	return b.String()
}
