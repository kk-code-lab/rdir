package state

import "strings"

func renderMarkdownSegments(doc markdownDocument) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	for idx, block := range doc.blocks {
		rendered := renderBlockSegments(block, 0)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && len(lines[len(lines)-1]) != 0 {
			lines = append(lines, nil)
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderBlockSegments(block markdownBlock, depth int) [][]StyledTextSegment {
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
		return renderListSegments(b, depth)
	case markdownBlockquote:
		return renderBlockquoteSegments(b, depth)
	case markdownHorizontalRule:
		return [][]StyledTextSegment{{{Text: "─", Style: TextStyleRule}}}
	case markdownTable:
		return renderTableSegments(b)
	default:
		return nil
	}
}

func renderParagraphSegments(inlines []markdownInline) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	var current []StyledTextSegment

	flush := func(force bool) {
		if len(current) == 0 && force {
			lines = append(lines, []StyledTextSegment{})
		} else if len(current) > 0 {
			lines = append(lines, current)
		}
		current = nil
	}

	for _, inline := range inlines {
		switch inline.kind {
		case inlineLineBreak:
			flush(true)
		case inlineText:
			current = append(current, StyledTextSegment{Text: inline.literal, Style: TextStylePlain})
		case inlineEmphasis:
			current = append(current, renderInlineSegments(inline.children, TextStyleEmphasis)...)
		case inlineStrong:
			current = append(current, renderInlineSegments(inline.children, TextStyleStrong)...)
		case inlineStrike:
			current = append(current, renderInlineSegments(inline.children, TextStyleStrike)...)
		case inlineCode:
			current = append(current, StyledTextSegment{Text: inline.literal, Style: TextStyleCode})
		case inlineLink:
			child := renderInlineSegments(inline.children, TextStyleLink)
			current = append(current, child...)
			if inline.destination != "" {
				current = append(current, StyledTextSegment{Text: " (", Style: TextStylePlain})
				current = append(current, StyledTextSegment{Text: inline.destination, Style: TextStyleLink})
				current = append(current, StyledTextSegment{Text: ")", Style: TextStylePlain})
			}
		case inlineImage:
			current = append(current, StyledTextSegment{Text: inline.literal, Style: TextStylePlain})
			if inline.destination != "" {
				current = append(current, StyledTextSegment{Text: " (", Style: TextStylePlain})
				current = append(current, StyledTextSegment{Text: inline.destination, Style: TextStyleLink})
				current = append(current, StyledTextSegment{Text: ")", Style: TextStylePlain})
			}
		}
	}
	flush(true)
	return lines
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

func renderListSegments(list markdownList, depth int) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	pad := strings.Repeat("  ", depth)
	for idx, item := range list.items {
		bullet := bulletSymbol(depth, list.ordered, idx, list.start)
		blocks := renderBlocksSegments(item.blocks, depth+1)
		if len(blocks) == 0 {
			lines = append(lines, []StyledTextSegment{{Text: pad + bullet, Style: TextStylePlain}})
			continue
		}
		first := append([]StyledTextSegment{{Text: pad + bullet + " ", Style: TextStylePlain}}, blocks[0]...)
		lines = append(lines, first)
		for _, line := range blocks[1:] {
			prefix := pad + strings.Repeat(" ", displayWidthStr(bullet)+1)
			lines = append(lines, append([]StyledTextSegment{{Text: prefix, Style: TextStylePlain}}, line...))
		}
	}
	return lines
}

func renderBlockquoteSegments(b markdownBlockquote, depth int) [][]StyledTextSegment {
	content := renderBlocksSegments(b.blocks, depth)
	if len(content) == 0 {
		return nil
	}
	withPrefix := make([][]StyledTextSegment, 0, len(content))
	for _, line := range content {
		withPrefix = append(withPrefix, append([]StyledTextSegment{{Text: "│ ", Style: TextStylePlain}}, line...))
	}
	return withPrefix
}

func renderBlocksSegments(blocks []markdownBlock, depth int) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	for idx, block := range blocks {
		rendered := renderBlockSegments(block, depth)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && len(lines[len(lines)-1]) != 0 {
			lines = append(lines, nil)
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderTableSegments(tbl markdownTable) [][]StyledTextSegment {
	if len(tbl.headers) == 0 {
		return nil
	}
	fancy := buildFormattedTable(tbl.headers, tbl.rows, tbl.align)
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
