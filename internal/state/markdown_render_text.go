package state

import "strings"

func renderMarkdown(doc markdownDocument) []string {
	var lines []string
	for idx, block := range doc.blocks {
		rendered := renderBlock(block, 0)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderBlock(block markdownBlock, depth int) []string {
	switch b := block.(type) {
	case markdownHeading:
		text := renderInlines(b.text)
		prefix := strings.Repeat("#", b.level)
		if prefix == "" {
			prefix = "#"
		}
		return []string{strings.TrimSpace(prefix + " " + text)}
	case markdownParagraph:
		return renderParagraphLines(b.text)
	case markdownCodeBlock:
		return renderCodeBlock(b)
	case markdownList:
		return renderList(b, depth)
	case markdownBlockquote:
		return renderBlockquote(b, depth)
	case markdownHorizontalRule:
		return []string{"─"}
	case markdownTable:
		return renderTable(b)
	default:
		return nil
	}
}

func renderParagraphLines(inlines []markdownInline) []string {
	var lines []string
	var b strings.Builder

	flush := func(force bool) {
		line := strings.TrimSpace(b.String())
		if force || line != "" || len(lines) == 0 {
			lines = append(lines, line)
		}
		b.Reset()
	}

	for _, inline := range inlines {
		switch inline.kind {
		case inlineLineBreak:
			flush(true)
		case inlineText:
			b.WriteString(inline.literal)
		case inlineEmphasis, inlineStrong, inlineStrike:
			b.WriteString(renderInlines(inline.children))
		case inlineCode:
			b.WriteString(inline.literal)
		case inlineLink:
			text := renderInlines(inline.children)
			if inline.destination != "" {
				b.WriteString(text)
				b.WriteString(" (")
				b.WriteString(inline.destination)
				b.WriteString(")")
			} else {
				b.WriteString(text)
			}
		case inlineImage:
			b.WriteString(inline.literal)
			if inline.destination != "" {
				b.WriteString(" (")
				b.WriteString(inline.destination)
				b.WriteString(")")
			}
		}
	}
	flush(true)
	return lines
}

func renderCodeBlock(block markdownCodeBlock) []string {
	if len(block.lines) == 0 {
		return []string{}
	}
	prefix := "    "
	lines := make([]string, 0, len(block.lines)+1)
	if block.info != "" {
		lines = append(lines, prefix+"["+block.info+"]")
	}
	for _, line := range block.lines {
		lines = append(lines, prefix+line)
	}
	return lines
}

func renderList(list markdownList, depth int) []string {
	var lines []string
	pad := strings.Repeat("  ", depth)
	for idx, item := range list.items {
		bullet := bulletSymbol(depth, list.ordered, idx, list.start)
		blocks := renderBlocks(item.blocks, depth+1)
		if len(blocks) == 0 {
			lines = append(lines, pad+bullet)
			continue
		}
		first := pad + bullet + " " + strings.TrimSpace(blocks[0])
		lines = append(lines, first)
		for _, line := range blocks[1:] {
			prefix := pad + strings.Repeat(" ", displayWidthStr(bullet)+1)
			lines = append(lines, prefix+line)
		}
	}
	return lines
}

func renderBlockquote(b markdownBlockquote, depth int) []string {
	content := renderBlocks(b.blocks, depth)
	if len(content) == 0 {
		return nil
	}
	withPrefix := make([]string, 0, len(content))
	for _, line := range content {
		withPrefix = append(withPrefix, "│ "+line)
	}
	return withPrefix
}

func renderBlocks(blocks []markdownBlock, depth int) []string {
	var lines []string
	for idx, block := range blocks {
		rendered := renderBlock(block, depth)
		if idx > 0 && len(rendered) > 0 && len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}
	return lines
}

func renderInlines(inlines []markdownInline) string {
	var builder strings.Builder
	for _, inline := range inlines {
		switch inline.kind {
		case inlineText:
			builder.WriteString(inline.literal)
		case inlineLineBreak:
			builder.WriteRune('\n')
		case inlineEmphasis, inlineStrong, inlineStrike:
			builder.WriteString(renderInlines(inline.children))
		case inlineCode:
			builder.WriteString(inline.literal)
		case inlineLink:
			text := renderInlines(inline.children)
			if inline.destination != "" {
				builder.WriteString(text)
				builder.WriteString(" (")
				builder.WriteString(inline.destination)
				builder.WriteString(")")
			} else {
				builder.WriteString(text)
			}
		case inlineImage:
			builder.WriteString(inline.literal)
			if inline.destination != "" {
				builder.WriteString(" (")
				builder.WriteString(inline.destination)
				builder.WriteString(")")
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func renderTable(tbl markdownTable) []string {
	if len(tbl.headers) == 0 {
		return nil
	}
	fancy := buildFormattedTable(tbl.headers, tbl.rows, tbl.align)
	return fancy.rows
}
