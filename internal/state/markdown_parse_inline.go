package state

import (
	"strings"
	"unicode"
)

func parseInline(text string) []markdownInline {
	runes := []rune(text)
	var nodes []markdownInline
	var buf []rune

	flushText := func() {
		if len(buf) == 0 {
			return
		}
		nodes = append(nodes, markdownInline{kind: inlineText, literal: string(buf)})
		buf = buf[:0]
	}

	i := 0
	for i < len(runes) {
		r := runes[i]
		switch r {
		case '\\':
			if i+1 < len(runes) {
				buf = append(buf, runes[i+1])
				i += 2
			} else {
				buf = append(buf, r)
				i++
			}
		case '\n':
			flushText()
			nodes = append(nodes, markdownInline{kind: inlineLineBreak})
			i++
		case '`':
			count := countRepeat(runes[i:], '`')
			end := findClosingBackticks(runes[i+count:], count)
			if end == -1 {
				buf = append(buf, runes[i:i+count]...)
				i += count
				continue
			}
			flushText()
			code := string(runes[i+count : i+count+end])
			nodes = append(nodes, markdownInline{kind: inlineCode, literal: code})
			i += count + end + count
		case '!':
			if i+1 < len(runes) && runes[i+1] == '[' {
				node, consumed, ok := parseLinkOrImage(runes[i:], true)
				if ok {
					flushText()
					nodes = append(nodes, node)
					i += consumed
					continue
				}
			}
			buf = append(buf, r)
			i++
		case '[':
			node, consumed, ok := parseLinkOrImage(runes[i:], false)
			if ok {
				flushText()
				nodes = append(nodes, node)
				i += consumed
				continue
			}
			buf = append(buf, r)
			i++
		case '<':
			if brLen := detectBreakTag(runes[i:]); brLen > 0 {
				flushText()
				nodes = append(nodes, markdownInline{kind: inlineLineBreak})
				i += brLen
				continue
			}
			end := findAutolinkEnd(runes[i+1:])
			if end >= 0 {
				candidate := string(runes[i+1 : i+1+end])
				if isAutolink(candidate) {
					flushText()
					display := candidate
					if strings.HasPrefix(candidate, "mailto:") {
						display = strings.TrimPrefix(candidate, "mailto:")
					}
					nodes = append(nodes, markdownInline{
						kind:        inlineLink,
						children:    []markdownInline{{kind: inlineText, literal: display}},
						destination: candidate,
					})
					i += end + 2
					continue
				}
			}
			buf = append(buf, r)
			i++
		case '*', '_':
			run := countRepeat(runes[i:], r)
			if run >= 2 {
				run = 2
			} else {
				run = 1
			}
			closeIdx := findClosingDelimiter(runes, i+run, r, run)
			if closeIdx == -1 {
				buf = append(buf, r)
				i++
				continue
			}
			if isAlnum(runes, i-1) && isAlnum(runes, closeIdx+run) {
				buf = append(buf, r)
				i++
				continue
			}
			flushText()
			content := parseInline(string(runes[i+run : closeIdx]))
			kind := inlineEmphasis
			if run == 2 {
				kind = inlineStrong
			}
			nodes = append(nodes, markdownInline{kind: kind, children: content})
			i = closeIdx + run
		case '~':
			run := countRepeat(runes[i:], r)
			if run < 2 {
				buf = append(buf, r)
				i++
				continue
			}
			closeIdx := findClosingDelimiter(runes, i+run, r, run)
			if closeIdx == -1 {
				buf = append(buf, r)
				i++
				continue
			}
			flushText()
			content := parseInline(string(runes[i+run : closeIdx]))
			nodes = append(nodes, markdownInline{kind: inlineStrike, children: content})
			i = closeIdx + run
		default:
			buf = append(buf, r)
			i++
		}
	}

	if len(buf) > 0 {
		nodes = append(nodes, markdownInline{kind: inlineText, literal: string(buf)})
	}
	return nodes
}

func parseLinkOrImage(runes []rune, isImage bool) (markdownInline, int, bool) {
	offset := 0
	if isImage {
		offset = 1
	}

	endText := findMatchingBracket(runes[offset+1:])
	if endText == -1 || offset+1+endText+1 >= len(runes) || runes[offset+1+endText+1] != '(' {
		return markdownInline{}, 0, false
	}

	closeParen, ok := findMatchingParen(runes[offset+1+endText+2:])
	if !ok {
		return markdownInline{}, 0, false
	}

	textEnd := offset + 1 + endText
	textRunes := runes[offset+1 : textEnd]
	destStart := textEnd + 2
	destRunes := runes[destStart : destStart+closeParen]
	dest := strings.TrimSpace(string(destRunes))

	if isImage {
		return markdownInline{
			kind:        inlineImage,
			literal:     string(textRunes),
			destination: dest,
		}, destStart + closeParen + 1, true
	}

	children := parseInline(string(textRunes))
	return markdownInline{
		kind:        inlineLink,
		children:    children,
		destination: dest,
	}, destStart + closeParen + 1, true
}

func findMatchingBracket(runes []rune) int {
	depth := 0
	for i := 0; i < len(runes); {
		switch r := runes[i]; r {
		case '\\':
			i += 2
			continue
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return i
			}
			depth--
		}
		i++
	}
	return -1
}

func findMatchingParen(runes []rune) (int, bool) {
	depth := 0
	for i := 0; i < len(runes); {
		switch r := runes[i]; r {
		case '\\':
			i += 2
			continue
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return i, true
			}
			depth--
		}
		i++
	}
	return -1, false
}

func findClosingBackticks(runes []rune, count int) int {
	for i := 0; i < len(runes); i++ {
		if runes[i] != '`' {
			continue
		}
		if countRepeat(runes[i:], '`') == count {
			return i
		}
	}
	return -1
}

func findClosingDelimiter(runes []rune, start int, delim rune, count int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] != delim {
			continue
		}
		if countRepeat(runes[i:], delim) < count {
			continue
		}
		if i > 0 && runes[i-1] == '\\' {
			continue
		}
		return i
	}
	return -1
}

func isAlnum(runes []rune, idx int) bool {
	if idx < 0 || idx >= len(runes) {
		return false
	}
	return unicode.IsLetter(runes[idx]) || unicode.IsDigit(runes[idx])
}

func countRepeat(runes []rune, target rune) int {
	n := 0
	for n < len(runes) && runes[n] == target {
		n++
	}
	return n
}

func findAutolinkEnd(runes []rune) int {
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' {
			i++
			continue
		}
		if r == '>' {
			return i
		}
	}
	return -1
}

func isAutolink(s string) bool {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return true
	}
	if strings.Contains(s, "@") && strings.Contains(s, ".") {
		return true
	}
	return false
}

func detectBreakTag(runes []rune) int {
	lower := strings.ToLower(string(runes))
	switch {
	case strings.HasPrefix(lower, "<br>"):
		return len("<br>")
	case strings.HasPrefix(lower, "<br/>"):
		return len("<br/>")
	case strings.HasPrefix(lower, "<br />"):
		return len("<br />")
	default:
		return 0
	}
}
