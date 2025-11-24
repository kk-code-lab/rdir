package state

// TextStyleKind describes a semantic style for formatted preview segments.
type TextStyleKind int

const (
	TextStylePlain TextStyleKind = iota
	TextStyleEmphasis
	TextStyleStrong
	TextStyleStrike
	TextStyleCode
	TextStyleCodeBlock
	TextStyleLink
	TextStyleHeading
	TextStyleRule
)

// StyledTextSegment is a chunk of text with an associated style.
type StyledTextSegment struct {
	Text  string
	Style TextStyleKind
}

func joinSegmentsText(segments []StyledTextSegment) string {
	if len(segments) == 0 {
		return ""
	}
	total := 0
	for _, seg := range segments {
		total += len(seg.Text)
	}
	buf := make([]byte, 0, total)
	for _, seg := range segments {
		buf = append(buf, seg.Text...)
	}
	return string(buf)
}

func deepCopySegments(lines [][]StyledTextSegment) [][]StyledTextSegment {
	if len(lines) == 0 {
		return nil
	}
	out := make([][]StyledTextSegment, len(lines))
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		copied := make([]StyledTextSegment, len(line))
		copy(copied, line)
		out[i] = copied
	}
	return out
}
