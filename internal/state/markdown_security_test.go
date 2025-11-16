package state

import "testing"

func TestSanitizeLinkDestinationBlocksUnsafeSchemes(t *testing.T) {
	if _, ok := sanitizeLinkDestination("javascript:alert(1)"); ok {
		t.Fatalf("expected javascript scheme to be rejected")
	}
	if _, ok := sanitizeLinkDestination("//example.com"); ok {
		t.Fatalf("expected protocol-relative link to be rejected")
	}
	if _, ok := sanitizeLinkDestination("http://example.com/path"); !ok {
		t.Fatalf("expected http link to be allowed")
	}
	if _, ok := sanitizeLinkDestination("mailto:test@example.com"); !ok {
		t.Fatalf("expected mailto link to be allowed")
	}
	poison := "http://example.com/" + string(rune(0x202E))
	if _, ok := sanitizeLinkDestination(poison); ok {
		t.Fatalf("expected formatting control runes to be rejected")
	}
}

func TestParseInlineRespectsRecursionLimit(t *testing.T) {
	text := "content"
	nodes := parseInlineDepth(text, inlineRecursionLimit)
	if len(nodes) != 1 || nodes[0].kind != inlineText || nodes[0].literal != text {
		t.Fatalf("expected recursion limit to return single text node, got %#v", nodes)
	}
}

func TestParseBlocksDepthLimit(t *testing.T) {
	lines := []string{"content"}
	blocks, next := parseBlocksWithDepth(lines, 0, markdownNestingLimit)
	if len(blocks) != 1 {
		t.Fatalf("expected a single paragraph block at limit, got %d", len(blocks))
	}
	if next != len(lines) {
		t.Fatalf("expected parser to advance to end, got %d", next)
	}
	if para, ok := blocks[0].(markdownParagraph); ok {
		rendered := renderInlineSegments(para.text, TextStylePlain)
		if got := joinSegmentsText(rendered); got != "content" {
			t.Fatalf("unexpected paragraph content %q", got)
		}
	} else {
		t.Fatalf("expected paragraph block, got %T", blocks[0])
	}
}
