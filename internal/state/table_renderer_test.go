package state

import (
	"testing"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

func TestTableLayoutClampsWidths(t *testing.T) {
	headers := [][]markdownInline{
		{{kind: inlineText, literal: "12345678"}},
	}
	opts := tableRenderOptions{MaxWidth: 10}

	layout := buildTableLayout(headers, nil, opts)

	if got := layout.widths[0]; got != 6 { // 8 -> clamped to fit borders (6 + 3 padding + 1 border = 10)
		t.Fatalf("expected column width to be clamped to 6, got %d", got)
	}
	if tw := tableWidth(layout.widths); tw > opts.MaxWidth {
		t.Fatalf("expected table width <= %d, got %d", opts.MaxWidth, tw)
	}
}

func TestTableRenderingRespectsMaxLinesPerCell(t *testing.T) {
	headers := [][]markdownInline{
		{{kind: inlineText, literal: "H"}},
	}
	rows := [][][]markdownInline{
		{{{kind: inlineText, literal: "abcdefghijklmn"}}},
	}
	align := []tableAlignment{alignDefault}
	opts := tableRenderOptions{
		MaxWidth:        9, // forces column width small enough to wrap
		MaxLinesPerCell: 1,
	}

	out := buildFormattedTable(headers, rows, align, opts)
	if len(out.rows) < 4 {
		t.Fatalf("unexpected table height: %d", len(out.rows))
	}
	body := out.rows[3] // first data row after top, header, separator
	if !containsEllipsis(body) {
		t.Fatalf("expected body row to be truncated with ellipsis, got %q", body)
	}
	if w := textutil.DisplayWidth(body); w > opts.MaxWidth {
		t.Fatalf("row exceeds max width: %d > %d", w, opts.MaxWidth)
	}
}

func TestTableCellWidthUsesGraphemeWidth(t *testing.T) {
	cell := makeTableCell([]markdownInline{
		{kind: inlineText, literal: "⚠️a"},
	}, TextStylePlain)
	if len(cell.lines) != 1 {
		t.Fatalf("expected single line, got %d", len(cell.lines))
	}
	want := textutil.DisplayWidth("⚠️a")
	if got := cell.lines[0].width; got != want {
		t.Fatalf("expected width %d, got %d", want, got)
	}
}

func containsEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}
