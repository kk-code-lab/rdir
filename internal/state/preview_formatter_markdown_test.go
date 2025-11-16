package state

import (
	"fmt"
	"strings"
	"testing"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
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
		{
			name: "inline pipe without table",
			line: "Value | with pipe characters",
			want: "Value | with pipe characters",
		},
		{
			name: "link with parentheses",
			line: "See [docs](https://example.com/foo(bar)/baz)",
			want: "See docs (https://example.com/foo(bar)/baz)",
		},
		{
			name: "underscore inside identifier unchanged",
			line: "config_map_value should stay intact",
			want: "config_map_value should stay intact",
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

func TestMarkdownHardLineBreaks(t *testing.T) {
	lines := []string{
		"First line  ",
		"Second line",
		"Line with \\",
		"backslash break",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"First line",
		"Second line Line with",
		"backslash break",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestMarkdownBreakTag(t *testing.T) {
	got := formatMarkdownLines([]string{"Hello<br>world"})
	want := []string{"Hello", "world"}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestHardLineBreaksInListsAndQuotes(t *testing.T) {
	lines := []string{
		"- item line  ",
		"  continues",
		"",
		"> quoted line  ",
		"> next",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"• item line",
		"  continues",
		"",
		"│ quoted line",
		"│ next",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
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

func TestTableSplittingRespectsEscapedPipes(t *testing.T) {
	lines := []string{
		"A | B",
		"---|---",
		"`code | span` \\| raw | value",
		"x \\| y | z",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"┌───────────────────┬───────┐",
		"│ A                 │ B     │",
		"├───────────────────┼───────┤",
		"│ code | span | raw │ value │",
		"│ x | y             │ z     │",
		"└───────────────────┴───────┘",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestTableRendersInlineContentAndLineBreaks(t *testing.T) {
	lines := []string{
		"| H1 | H2 |",
		"| --- | --- |",
		"| *em* and [l](url) | `code`<br>next |",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"┌────────────────┬──────┐",
		"│ H1             │ H2   │",
		"├────────────────┼──────┤",
		"│ em and l (url) │ code │",
		"│                │ next │",
		"└────────────────┴──────┘",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestTableKeepsEscapedPipeRow(t *testing.T) {
	lines := []string{
		"| Left align | Center align | Right align |",
		"| :--- | :---: | ---: |",
		"| left value | centered `code` | 42 |",
		"| link: [docs](https://example.com) | multi<br>line | 123456 |",
		"| \\| literal pipe | **bold** and _em_ | -7 |",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"┌──────────────────────────────────┬───────────────┬─────────────┐",
		"│ Left align                       │ Center align  │ Right align │",
		"├──────────────────────────────────┼───────────────┼─────────────┤",
		"│ left value                       │ centered code │          42 │",
		"│ link: docs (https://example.com) │     multi     │      123456 │",
		"│                                  │     line      │             │",
		"│ | literal pipe                   │  bold and em  │          -7 │",
		"└──────────────────────────────────┴───────────────┴─────────────┘",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestTableRespectsWidthAndWrapToggle(t *testing.T) {
	lines := []string{
		"| A | B |",
		"| --- | --- |",
		"| verylongvalue | anotherlongvalue |",
	}

	segs, _ := FormatMarkdownPreview(lines, 24, 1, false)
	var truncated string
	for _, line := range segs {
		text := joinSegmentsText(line)
		if strings.Contains(text, "very") {
			truncated = text
			break
		}
	}
	if truncated == "" {
		t.Fatalf("expected to find truncated row")
	}
	if !strings.Contains(truncated, "…") {
		t.Fatalf("expected ellipsis in truncated row, got %q", truncated)
	}
	if width := textutil.DisplayWidth(truncated); width > 24 {
		t.Fatalf("truncated row exceeds width: %d", width)
	}

	wrapped, _ := FormatMarkdownPreview(lines, 24, 0, true)
	if len(wrapped) <= len(segs) {
		t.Fatalf("expected wrapped output to have more lines, got %d vs %d", len(wrapped), len(segs))
	}
}

func TestInvalidTableDoesNotParse(t *testing.T) {
	lines := []string{
		"A | B | C",
		"----|----",
		"",
		"after",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"A | B | C ----|----",
		"",
		"after",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestMarkdownBlockquotePreservesBlankLines(t *testing.T) {
	lines := []string{
		"> first line",
		"",
		"> second line",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"│ first line",
		"│ ",
		"│ second line",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestMarkdownListWithFencedCodeBlock(t *testing.T) {
	lines := []string{
		"- item with code",
		"  ```go",
		"  fmt.Println(\"ok\")",
		"  ```",
		"  tail",
		"2. second top-level item",
	}

	got := formatMarkdownLines(lines)
	want := []string{
		"• item with code",
		"  ",
		"      [go]",
		"      fmt.Println(\"ok\")",
		"  ",
		"  tail",
		"",
		"2. second top-level item",
	}

	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("formatted markdown mismatch:\n%s", diff)
	}
}

func TestMarkdownPreviewFromDataUsesCachedDocument(t *testing.T) {
	content := "# Header\n\n| H1 | H2 |\n| --- | --- |\n| left | right |\n"
	info := fakeFileInfo{name: "cache.md", size: int64(len(content))}
	ctx := previewFormatContext{
		path:    info.Name(),
		info:    info,
		content: []byte(content),
	}

	preview := &PreviewData{}
	markdownPreviewFormatter{}.Format(ctx, preview)

	if preview.markdownDoc == nil {
		t.Fatalf("expected cached markdown document")
	}

	preview.TextLines = nil

	segments, meta := FormatMarkdownPreviewFromData(preview, 40, 1, false)
	if len(segments) == 0 {
		t.Fatalf("expected segments when rendering from cached document")
	}
	if len(meta) != len(segments) {
		t.Fatalf("expected metadata for each line, got %d vs %d", len(meta), len(segments))
	}

	lines := segmentsToTextLines(segments)
	if len(lines) == 0 || lines[0] != "# Header" {
		t.Fatalf("unexpected rendered output: %#v", lines)
	}
}

func TestMarkdownSegmentsMatchFormattedLines(t *testing.T) {
	lines := []string{
		"## Title",
		"",
		"Text with [link](https://example.com) and `code`.",
	}
	want := formatMarkdownLines(lines)

	segs, _ := FormatMarkdownPreview(lines, 120, 3, false)
	got := segmentsToTextLines(segs)
	if diff := diffLines(want, got); diff != "" {
		t.Fatalf("segments text mismatch:\n%s", diff)
	}

	var foundHeading, foundLink bool
	for _, line := range segs {
		for _, seg := range line {
			if seg.Style == TextStyleHeading {
				foundHeading = true
			}
			if seg.Style == TextStyleLink {
				foundLink = true
			}
		}
	}
	if !foundHeading || !foundLink {
		t.Fatalf("expected heading and link styles, got heading=%v link=%v", foundHeading, foundLink)
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
