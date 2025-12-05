package state

import (
	"strings"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"github.com/rivo/uniseg"
)

// formattedTable holds pre-rendered rows for fancy tables.
type formattedTable struct {
	rows        []string
	segmentRows [][]StyledTextSegment
	meta        []TextLineMetadata
}

type tableLayout struct {
	widths []int
	header []tableCell
	rows   [][]tableCell
}

type tableBorders struct {
	topLeft, topSep, topRight          string
	midLeft, midSep, midRight          string
	bottomLeft, bottomSep, bottomRight string
}

func defaultTableBorders() tableBorders {
	return tableBorders{
		topLeft:     "┌",
		topSep:      "┬",
		topRight:    "┐",
		midLeft:     "├",
		midSep:      "┼",
		midRight:    "┤",
		bottomLeft:  "└",
		bottomSep:   "┴",
		bottomRight: "┘",
	}
}

type tableRenderOptions struct {
	// MaxWidth clamps the total rendered width (in columns). Zero means unlimited.
	MaxWidth int
	// MaxLinesPerCell limits how many wrapped lines each cell may emit. Zero means unlimited.
	MaxLinesPerCell int
	// Ellipsis is appended when content is truncated. Defaults to "…" when empty.
	Ellipsis string
}

type tableCell struct {
	lines []cellLine
}

type cellLine struct {
	text     string
	segments []StyledTextSegment
	width    int
}

func buildFormattedTable(headers [][]markdownInline, rows [][][]markdownInline, align []tableAlignment, opts tableRenderOptions) formattedTable {
	if len(headers) == 0 {
		return formattedTable{}
	}

	layout := buildTableLayout(headers, rows, opts)
	return renderFormattedTable(layout, align, defaultTableBorders())
}

func buildTableLayout(headers [][]markdownInline, rows [][][]markdownInline, opts tableRenderOptions) tableLayout {
	headerRaw, rowRaw := prepareTableCells(headers, rows)

	widths := computeColumnWidths(headerRaw, rowRaw)
	widths = clampColumnWidths(widths, opts.MaxWidth)

	headerCells := wrapCellsToWidth(headerRaw, widths, opts)
	rowCells := wrapRowCells(rowRaw, widths, opts)

	return tableLayout{
		widths: widths,
		header: headerCells,
		rows:   rowCells,
	}
}

func prepareTableCells(headers [][]markdownInline, rows [][][]markdownInline) ([]tableCell, [][]tableCell) {
	headerRaw := make([]tableCell, len(headers))
	for i, h := range headers {
		headerRaw[i] = makeTableCell(h, TextStyleStrong)
	}

	rowRaw := make([][]tableCell, len(rows))
	for i, row := range rows {
		rowRaw[i] = make([]tableCell, len(headers))
		for j := range headers {
			if j < len(row) {
				rowRaw[i][j] = makeTableCell(row[j], TextStylePlain)
			} else {
				rowRaw[i][j] = makeTableCell(nil, TextStylePlain)
			}
		}
	}

	return headerRaw, rowRaw
}

func renderFormattedTable(layout tableLayout, align []tableAlignment, borders tableBorders) formattedTable {
	if len(layout.header) == 0 {
		return formattedTable{}
	}

	hCells := make([]string, len(layout.widths))
	for i := range layout.widths {
		hCells[i] = strings.Repeat("─", layout.widths[i]+2)
	}

	var lines []string
	var segmentRows [][]StyledTextSegment
	var meta []TextLineMetadata
	offset := int64(0)

	top := buildBorderLine(hCells, borders.topLeft, borders.topSep, borders.topRight)
	lines = append(lines, top)
	segmentRows = append(segmentRows, []StyledTextSegment{{Text: top, Style: TextStylePlain}})
	meta = append(meta, makeMeta(top, offset))
	offset += int64(len(top))

	headerHeight := cellBlockHeight(layout.header)
	for i := 0; i < headerHeight; i++ {
		headerLine, headerSegs := renderTableRow(layout.header, i, layout.widths, align)
		lines = append(lines, headerLine)
		segmentRows = append(segmentRows, headerSegs)
		meta = append(meta, makeMeta(headerLine, offset))
		offset += int64(len(headerLine))
	}

	separator := buildBorderLine(hCells, borders.midLeft, borders.midSep, borders.midRight)
	lines = append(lines, separator)
	segmentRows = append(segmentRows, []StyledTextSegment{{Text: separator, Style: TextStylePlain}})
	meta = append(meta, makeMeta(separator, offset))
	offset += int64(len(separator))

	for _, row := range layout.rows {
		rowHeight := cellBlockHeight(row)
		for i := 0; i < rowHeight; i++ {
			line, segs := renderTableRow(row, i, layout.widths, align)
			lines = append(lines, line)
			segmentRows = append(segmentRows, segs)
			meta = append(meta, makeMeta(line, offset))
			offset += int64(len(line))
		}
	}

	bottom := buildBorderLine(hCells, borders.bottomLeft, borders.bottomSep, borders.bottomRight)
	lines = append(lines, bottom)
	segmentRows = append(segmentRows, []StyledTextSegment{{Text: bottom, Style: TextStylePlain}})
	meta = append(meta, makeMeta(bottom, offset))

	return formattedTable{rows: lines, segmentRows: segmentRows, meta: meta}
}

func buildBorderLine(columns []string, left, sep, right string) string {
	return left + strings.Join(columns, sep) + right
}

func computeColumnWidths(headers []tableCell, rows [][]tableCell) []int {
	widths := make([]int, len(headers))
	update := func(cell tableCell, idx int) {
		for _, line := range cell.lines {
			if line.width > widths[idx] {
				widths[idx] = line.width
			}
		}
	}
	for i, cell := range headers {
		update(cell, i)
	}
	for _, row := range rows {
		for i := range widths {
			if i < len(row) {
				update(row[i], i)
			}
		}
	}
	return widths
}

func clampColumnWidths(widths []int, maxWidth int) []int {
	if maxWidth <= 0 || len(widths) == 0 {
		return widths
	}
	const minColWidth = 3
	total := tableWidth(widths)
	for total > maxWidth {
		idx := widestColumn(widths, minColWidth)
		if idx == -1 {
			break
		}
		widths[idx]--
		total--
	}
	return widths
}

func wrapCellsToWidth(cells []tableCell, widths []int, opts tableRenderOptions) []tableCell {
	out := make([]tableCell, len(cells))
	for i, cell := range cells {
		target := 0
		if i < len(widths) {
			target = widths[i]
		}
		out[i] = wrapCellLines(cell, target, opts)
	}
	return out
}

func wrapRowCells(rows [][]tableCell, widths []int, opts tableRenderOptions) [][]tableCell {
	out := make([][]tableCell, len(rows))
	for i, row := range rows {
		out[i] = wrapCellsToWidth(row, widths, opts)
	}
	return out
}

func wrapCellLines(cell tableCell, width int, opts tableRenderOptions) tableCell {
	if width <= 0 {
		width = 1
	}
	var wrapped []cellLine
	for _, line := range cell.lines {
		segs := wrapSegmentsToWidth(line.segments, width)
		for _, segLine := range segs {
			text := joinSegmentsText(segLine)
			wrapped = append(wrapped, cellLine{
				text:     text,
				segments: segLine,
				width:    textutil.DisplayWidth(text),
			})
		}
	}
	if len(wrapped) == 0 {
		wrapped = []cellLine{{text: "", segments: nil, width: 0}}
	}

	if opts.MaxLinesPerCell > 0 && len(wrapped) > opts.MaxLinesPerCell {
		wrapped = wrapped[:opts.MaxLinesPerCell]
		ellipsis := opts.Ellipsis
		if ellipsis == "" {
			ellipsis = "…"
		}
		last := wrapped[len(wrapped)-1]
		wrapped[len(wrapped)-1] = trimLineToWidth(last, width, ellipsis)
	}

	return tableCell{lines: wrapped}
}

func trimLineToWidth(line cellLine, width int, ellipsis string) cellLine {
	if width <= 0 {
		return cellLine{text: "", width: 0}
	}
	ellWidth := textutil.DisplayWidth(ellipsis)
	if ellWidth >= width {
		return cellLine{text: ellipsis, segments: []StyledTextSegment{{Text: ellipsis, Style: TextStylePlain}}, width: ellWidth}
	}
	target := width - ellWidth
	var segs []StyledTextSegment
	curWidth := 0
	for _, seg := range line.segments {
		if curWidth >= target {
			break
		}
		var buf strings.Builder
		g := uniseg.NewGraphemes(seg.Text)
		for g.Next() {
			cluster := g.Str()
			w := textutil.DisplayWidth(cluster)
			if w < 1 {
				w = 1
			}
			if curWidth+w > target {
				break
			}
			buf.WriteString(cluster)
			curWidth += w
		}
		if buf.Len() > 0 {
			segs = append(segs, StyledTextSegment{Text: buf.String(), Style: seg.Style})
		}
	}
	segs = append(segs, StyledTextSegment{Text: ellipsis, Style: TextStylePlain})
	text := joinSegmentsText(segs)
	return cellLine{
		text:     text,
		segments: segs,
		width:    textutil.DisplayWidth(text),
	}
}

func wrapSegmentsToWidth(segments []StyledTextSegment, width int) [][]StyledTextSegment {
	if width <= 0 {
		return [][]StyledTextSegment{segments}
	}
	var lines [][]StyledTextSegment
	var current []StyledTextSegment
	currentWidth := 0

	flush := func() {
		line := make([]StyledTextSegment, len(current))
		copy(line, current)
		lines = append(lines, line)
		current = current[:0]
		currentWidth = 0
	}

	for _, seg := range segments {
		text := seg.Text
		if text == "" {
			continue
		}
		var buf strings.Builder
		g := uniseg.NewGraphemes(text)
		for g.Next() {
			cluster := g.Str()
			w := textutil.DisplayWidth(cluster)
			if w < 1 {
				w = 1
			}
			if currentWidth > 0 && currentWidth+w > width {
				current = append(current, StyledTextSegment{Text: buf.String(), Style: seg.Style})
				buf.Reset()
				flush()
			}
			if w > width {
				continue
			}
			buf.WriteString(cluster)
			currentWidth += w
			if currentWidth == width {
				current = append(current, StyledTextSegment{Text: buf.String(), Style: seg.Style})
				buf.Reset()
				flush()
			}
		}
		if buf.Len() > 0 {
			current = append(current, StyledTextSegment{Text: buf.String(), Style: seg.Style})
		}
	}
	if len(current) > 0 {
		flush()
	}
	if len(lines) == 0 {
		lines = append(lines, []StyledTextSegment{})
	}
	return lines
}

func widestColumn(widths []int, minWidth int) int {
	maxIdx := -1
	maxVal := minWidth
	for i, w := range widths {
		if w > maxVal {
			maxVal = w
			maxIdx = i
		}
	}
	return maxIdx
}

func tableWidth(widths []int) int {
	if len(widths) == 0 {
		return 0
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	// Each column gets 2 spaces + 1 border, plus one extra border at the end.
	return total + len(widths)*3 + 1
}

func renderTableRow(cells []tableCell, lineIdx int, widths []int, align []tableAlignment) (string, []StyledTextSegment) {
	var b strings.Builder
	var segments []StyledTextSegment

	writePlain := func(text string) {
		b.WriteString(text)
		segments = append(segments, StyledTextSegment{Text: text, Style: TextStylePlain})
	}

	writePlain("│ ")
	for i, cell := range cells {
		var line cellLine
		if lineIdx < len(cell.lines) {
			line = cell.lines[lineIdx]
		}
		text, segs := alignCell(line, widths[i], alignAt(i, align))
		b.WriteString(text)
		segments = append(segments, segs...)
		if i == len(cells)-1 {
			writePlain(" │")
		} else {
			writePlain(" │ ")
		}
	}
	return b.String(), segments
}

func alignCell(line cellLine, width int, alignment tableAlignment) (string, []StyledTextSegment) {
	space := width - line.width
	if space < 0 {
		space = 0
	}
	left, right := 0, space
	switch alignment {
	case alignCenter:
		left = space / 2
		right = space - left
	case alignRight:
		left = space
		right = 0
	}

	leftPad := strings.Repeat(" ", left)
	rightPad := strings.Repeat(" ", right)

	text := leftPad + line.text + rightPad
	segments := make([]StyledTextSegment, 0, 2+len(line.segments))
	if left > 0 {
		segments = append(segments, StyledTextSegment{Text: leftPad, Style: TextStylePlain})
	}
	segments = append(segments, line.segments...)
	if right > 0 {
		segments = append(segments, StyledTextSegment{Text: rightPad, Style: TextStylePlain})
	}
	return text, segments
}

func alignAt(idx int, align []tableAlignment) tableAlignment {
	if idx < len(align) {
		return align[idx]
	}
	return alignDefault
}

func makeMeta(line string, offset int64) TextLineMetadata {
	return TextLineMetadata{
		Offset:       offset,
		Length:       len(line),
		DisplayWidth: textutil.DisplayWidth(line),
		RuneCount:    len([]rune(line)),
	}
}

func makeTableCell(inlines []markdownInline, defaultStyle TextStyleKind) tableCell {
	lines := renderInlineLines(inlines, defaultStyle, nil)
	cellLines := make([]cellLine, len(lines))
	for i, line := range lines {
		expanded := expandTabsInSegments(line, textutil.DefaultTabWidth)
		text := joinSegmentsText(expanded)
		cellLines[i] = cellLine{
			text:     text,
			segments: expanded,
			width:    textutil.DisplayWidth(text),
		}
	}
	if len(cellLines) == 0 {
		cellLines = []cellLine{{text: "", segments: nil, width: 0}}
	}
	return tableCell{lines: cellLines}
}

func expandTabsInSegments(segments []StyledTextSegment, tabWidth int) []StyledTextSegment {
	if tabWidth <= 0 {
		return segments
	}
	if len(segments) == 0 {
		return nil
	}
	out := make([]StyledTextSegment, 0, len(segments))
	column := 0

	for _, seg := range segments {
		text := seg.Text
		if !strings.ContainsRune(text, '\t') {
			column += textutil.DisplayWidth(text)
			out = append(out, seg)
			continue
		}

		var b strings.Builder
		for _, r := range text {
			if r == '\t' {
				spaces := tabWidth - (column % tabWidth)
				if spaces == 0 {
					spaces = tabWidth
				}
				b.WriteString(strings.Repeat(" ", spaces))
				column += spaces
				continue
			}
			b.WriteRune(r)
			w := textutil.DisplayWidth(string(r))
			if w < 1 {
				w = 1
			}
			column += w
		}
		out = append(out, StyledTextSegment{Text: b.String(), Style: seg.Style})
	}
	return out
}

func cellBlockHeight(cells []tableCell) int {
	max := 0
	for _, cell := range cells {
		if len(cell.lines) > max {
			max = len(cell.lines)
		}
	}
	if max == 0 {
		return 1
	}
	return max
}
