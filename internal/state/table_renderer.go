package state

import (
	"strings"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

// formattedTable holds pre-rendered rows for fancy tables.
type formattedTable struct {
	rows []string
	meta []TextLineMetadata
}

func buildFormattedTable(headers []string, rows [][]string, align []tableAlignment) formattedTable {
	if len(headers) == 0 {
		return formattedTable{}
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		if l := textutil.DisplayWidth(h); l > widths[i] {
			widths[i] = l
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			if l := textutil.DisplayWidth(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}

	borderTopLeft, borderTopSep, borderTopRight := "┌", "┬", "┐"
	borderMidLeft, borderMidSep, borderMidRight := "├", "┼", "┤"
	borderBottomLeft, borderBottomSep, borderBottomRight := "└", "┴", "┘"
	borderVert := "│"

	hCells := make([]string, len(headers))
	for i := range headers {
		hCells[i] = strings.Repeat("─", widths[i]+2)
	}

	var lines []string
	var meta []TextLineMetadata
	offset := int64(0)

	top := borderTopLeft + strings.Join(hCells, borderTopSep) + borderTopRight
	lines = append(lines, top)
	meta = append(meta, makeMeta(top, offset))
	offset += int64(len(top))

	headerLine := borderVert + " " + strings.Join(applyAlignment(headers, widths, align), " "+borderVert+" ") + " " + borderVert
	lines = append(lines, headerLine)
	meta = append(meta, makeMeta(headerLine, offset))
	offset += int64(len(headerLine))

	separator := borderMidLeft + strings.Join(hCells, borderMidSep) + borderMidRight
	lines = append(lines, separator)
	meta = append(meta, makeMeta(separator, offset))
	offset += int64(len(separator))

	for _, row := range rows {
		line := borderVert + " " + strings.Join(applyAlignment(row, widths, align), " "+borderVert+" ") + " " + borderVert
		lines = append(lines, line)
		meta = append(meta, makeMeta(line, offset))
		offset += int64(len(line))
	}

	bottom := borderBottomLeft + strings.Join(hCells, borderBottomSep) + borderBottomRight
	lines = append(lines, bottom)
	meta = append(meta, makeMeta(bottom, offset))

	return formattedTable{rows: lines, meta: meta}
}

func applyAlignment(cells []string, widths []int, align []tableAlignment) []string {
	out := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		space := widths[i] - textutil.DisplayWidth(cell)
		if space < 0 {
			space = 0
		}
		left, right := 0, space
		if i < len(align) {
			switch align[i] {
			case alignCenter:
				left = space / 2
				right = space - left
			case alignRight:
				left = space
				right = 0
			default:
			}
		}
		out[i] = strings.Repeat(" ", left) + cell + strings.Repeat(" ", right)
	}
	return out
}

func makeMeta(line string, offset int64) TextLineMetadata {
	return TextLineMetadata{
		Offset:       offset,
		Length:       len(line),
		DisplayWidth: textutil.DisplayWidth(line),
		RuneCount:    len([]rune(line)),
	}
}
