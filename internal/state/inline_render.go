package state

func renderInlineLines(inlines []markdownInline, defaultStyle TextStyleKind, emptyLineOnForce []StyledTextSegment) [][]StyledTextSegment {
	var lines [][]StyledTextSegment
	var current []StyledTextSegment

	flush := func(force bool) {
		if len(current) == 0 {
			if force {
				lines = append(lines, emptyLineOnForce)
			}
		} else {
			lines = append(lines, current)
		}
		current = nil
	}

	for _, inline := range inlines {
		switch inline.kind {
		case inlineLineBreak:
			flush(true)
		case inlineText:
			current = append(current, StyledTextSegment{Text: inline.literal, Style: defaultStyle})
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
