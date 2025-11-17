package state

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func renderMarkdownSegments(doc markdownDocument, opts markdownRenderOptions) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	for idx, block := range doc.blocks {
		rendered := renderBlockSegments(block, 0, opts)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && len(lines[len(lines)-1]) != 0 {
			lines = append(lines, nil)
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderBlockSegments(block markdownBlock, depth int, opts markdownRenderOptions) [][]StyledTextSegment {
	switch b := block.(type) {
	case markdownHeading:
		textSegments := renderInlineSegments(b.text, TextStyleHeading)
		prefix := strings.Repeat("#", b.level)
		if prefix == "" {
			prefix = "#"
		}
		line := []StyledTextSegment{{Text: prefix + " ", Style: TextStyleHeading}}
		line = append(line, textSegments...)
		return [][]StyledTextSegment{line}
	case markdownParagraph:
		return renderParagraphSegments(b.text)
	case markdownCodeBlock:
		return renderCodeBlockSegments(b)
	case markdownList:
		return renderListSegments(b, depth, opts)
	case markdownBlockquote:
		return renderBlockquoteSegments(b, depth, opts)
	case markdownHorizontalRule:
		return [][]StyledTextSegment{{{Text: "─", Style: TextStyleRule}}}
	case markdownTable:
		return renderTableSegments(b, opts.tableOpts)
	default:
		return nil
	}
}

func renderParagraphSegments(inlines []markdownInline) [][]StyledTextSegment {
	return renderInlineLines(inlines, TextStylePlain, []StyledTextSegment{})
}

func renderCodeBlockSegments(block markdownCodeBlock) [][]StyledTextSegment {
	if len(block.lines) == 0 {
		return [][]StyledTextSegment{}
	}
	prefix := "    "
	lines := make([][]StyledTextSegment, 0, len(block.lines)+1)
	if block.info != "" {
		lines = append(lines, []StyledTextSegment{{Text: prefix + "[" + block.info + "]", Style: TextStyleCode}})
	}
	for _, line := range block.lines {
		lines = append(lines, []StyledTextSegment{{Text: prefix + line, Style: TextStyleCode}})
	}
	return lines
}

func renderListSegments(list markdownList, depth int, opts markdownRenderOptions) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	addLine := func(line []StyledTextSegment) {
		if line == nil {
			if len(lines) == 0 || lines[len(lines)-1] != nil {
				lines = append(lines, nil)
			}
			return
		}
		lines = append(lines, line)
	}

	pad := strings.Repeat("  ", depth)
	prevHadBody := false
	for idx, item := range list.items {
		bullet := bulletSymbol(depth, list.ordered, idx, list.start)
		blocks := renderBlocksSegments(item.blocks, depth+1, opts)
		hadBody := len(blocks) > 1
		if idx > 0 && prevHadBody {
			addLine(nil)
		}
		if len(blocks) == 0 {
			addLine([]StyledTextSegment{{Text: pad + bullet, Style: TextStylePlain}})
			prevHadBody = false
			continue
		}
		first := append([]StyledTextSegment{{Text: pad + bullet + " ", Style: TextStylePlain}}, blocks[0]...)
		addLine(first)
		if hadBody {
			addLine(nil)
		}
		for i := 1; i < len(blocks); i++ {
			line := blocks[i]
			if line != nil {
				text := strings.TrimLeft(joinSegmentsText(line), " ")
				if startsListMarker(text) {
					addLine(line)
					continue
				}
				prefix := pad + strings.Repeat(" ", displayWidthStr(bullet)+1)
				line = append([]StyledTextSegment{{Text: prefix, Style: TextStylePlain}}, line...)
			}
			addLine(line)
		}
		prevHadBody = hadBody
	}
	return lines
}

func startsListMarker(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	switch r {
	case '•', '◦', '▪':
		return true
	}
	return unicode.IsDigit(r)
}

func renderBlockquoteSegments(b markdownBlockquote, depth int, opts markdownRenderOptions) [][]StyledTextSegment {
	content := renderBlocksSegments(b.blocks, depth, opts)
	if len(content) == 0 {
		return nil
	}
	withPrefix := make([][]StyledTextSegment, 0, len(content))
	for _, line := range content {
		withPrefix = append(withPrefix, append([]StyledTextSegment{{Text: "│ ", Style: TextStylePlain}}, line...))
	}
	return withPrefix
}

func renderBlocksSegments(blocks []markdownBlock, depth int, opts markdownRenderOptions) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	for idx, block := range blocks {
		rendered := renderBlockSegments(block, depth, opts)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && len(lines[len(lines)-1]) != 0 &&
			(depth == 0 || !blockIsList(block)) {
			lines = append(lines, nil)
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func blockIsList(block markdownBlock) bool {
	_, ok := block.(markdownList)
	return ok
}

func renderTableSegments(tbl markdownTable, opts tableRenderOptions) [][]StyledTextSegment {
	if len(tbl.headers) == 0 {
		return nil
	}
	fancy := buildFormattedTable(tbl.headers, tbl.rows, tbl.align, opts)
	if len(fancy.segmentRows) > 0 {
		return fancy.segmentRows
	}
	segments := make([][]StyledTextSegment, len(fancy.rows))
	for i, row := range fancy.rows {
		segments[i] = []StyledTextSegment{{Text: row, Style: TextStylePlain}}
	}
	return segments
}

func renderInlineSegments(inlines []markdownInline, defaultStyle TextStyleKind) []StyledTextSegment {
	var segments []StyledTextSegment
	for _, inline := range inlines {
		switch inline.kind {
		case inlineText:
			segments = append(segments, StyledTextSegment{Text: inline.literal, Style: defaultStyle})
		case inlineEmphasis:
			child := renderInlineSegments(inline.children, TextStyleEmphasis)
			segments = append(segments, child...)
		case inlineStrong:
			child := renderInlineSegments(inline.children, TextStyleStrong)
			segments = append(segments, child...)
		case inlineStrike:
			child := renderInlineSegments(inline.children, TextStyleStrike)
			segments = append(segments, child...)
		case inlineCode:
			segments = append(segments, StyledTextSegment{Text: inline.literal, Style: TextStyleCode})
		case inlineLink:
			child := renderInlineSegments(inline.children, TextStyleLink)
			segments = append(segments, child...)
			if inline.destination != "" {
				segments = append(segments, StyledTextSegment{Text: " (", Style: TextStylePlain})
				segments = append(segments, StyledTextSegment{Text: inline.destination, Style: TextStyleLink})
				segments = append(segments, StyledTextSegment{Text: ")", Style: TextStylePlain})
			}
		case inlineImage:
			segments = append(segments, StyledTextSegment{Text: inline.literal, Style: TextStylePlain})
			if inline.destination != "" {
				segments = append(segments, StyledTextSegment{Text: " (", Style: TextStylePlain})
				segments = append(segments, StyledTextSegment{Text: inline.destination, Style: TextStyleLink})
				segments = append(segments, StyledTextSegment{Text: ")", Style: TextStylePlain})
			}
		}
	}
	return segments
}
