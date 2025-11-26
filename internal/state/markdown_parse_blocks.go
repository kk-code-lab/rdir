package state

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const markdownNestingLimit = 64
const fencedCodeIndentLimit = 3

func parseMarkdown(lines []string) markdownDocument {
	blocks, _ := parseBlocks(lines, 0)
	return markdownDocument{blocks: blocks}
}

func parseBlocks(lines []string, start int) ([]markdownBlock, int) {
	return parseBlocksWithDepth(lines, start, 0)
}

func parseBlocksWithDepth(lines []string, start int, depth int) ([]markdownBlock, int) {
	var blocks []markdownBlock
	i := start
	if depth >= markdownNestingLimit {
		text := joinParagraphLines(lines[start:])
		return []markdownBlock{
			markdownParagraph{text: parseInline(text)},
		}, len(lines)
	}
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			i++
			continue
		}

		indent := leadingSpaces(line)
		trimmed := strings.TrimLeft(line, " \t")

		if tbl, next, ok := parseTable(lines, i); ok {
			blocks = append(blocks, tbl)
			i = next
			continue
		}

		if indent >= 4 {
			codeBlock, next := parseIndentedCodeBlock(lines, i)
			blocks = append(blocks, codeBlock)
			i = next
			continue
		}

		if fence, ok := detectFence(trimmed); ok {
			codeBlock, next := parseFencedCodeBlock(lines, i, fence)
			blocks = append(blocks, codeBlock)
			i = next
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			blockquote, next := parseBlockquote(lines, i, depth)
			blocks = append(blocks, blockquote)
			i = next
			continue
		}

		if headingLevel, headingText, ok := parseHeading(trimmed); ok {
			blocks = append(blocks, markdownHeading{
				level: headingLevel,
				text:  parseInline(headingText),
			})
			i++
			continue
		}

		if isHorizontalRule(trimmed) {
			blocks = append(blocks, markdownHorizontalRule{})
			i++
			continue
		}

		if level, ok := parseSetextHeading(lines, i); ok {
			text := strings.TrimSpace(lines[i])
			blocks = append(blocks, markdownHeading{
				level: level,
				text:  parseInline(text),
			})
			i += 2
			continue
		}

		if list, next, ok := parseList(lines, i, depth); ok {
			blocks = append(blocks, list)
			i = next
			continue
		}

		paragraph, next := parseParagraph(lines, i)
		blocks = append(blocks, paragraph)
		i = next
	}
	return blocks, i
}

func parseParagraph(lines []string, start int) (markdownParagraph, int) {
	var parts []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			break
		}

		indent := leadingSpaces(line)
		if indent >= 4 || startsBlock(lines, i) {
			break
		}
		parts = append(parts, line)
		i++
	}

	text := joinParagraphLines(parts)
	return markdownParagraph{text: parseInline(text)}, i
}

func parseBlockquote(lines []string, start int, depth int) (markdownBlockquote, int) {
	var quoteLines []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			quoteLines = append(quoteLines, "")
			i++
			continue
		}

		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, ">") {
			break
		}
		stripped := strings.TrimLeft(strings.TrimPrefix(trimmed, ">"), " \t")
		quoteLines = append(quoteLines, stripped)
		i++
	}

	children, _ := parseBlocksWithDepth(quoteLines, 0, depth+1)
	return markdownBlockquote{blocks: children}, i
}

type fenceSpec struct {
	delimiter rune
	length    int
	info      string
}

func detectFence(trimmed string) (fenceSpec, bool) {
	if trimmed == "" {
		return fenceSpec{}, false
	}
	first, size := utf8.DecodeRuneInString(trimmed)
	if size == 0 {
		return fenceSpec{}, false
	}
	if first != '`' && first != '~' {
		return fenceSpec{}, false
	}

	count := countRepeatRune(trimmed, first)
	if count < 3 {
		return fenceSpec{}, false
	}
	info := strings.TrimSpace(trimmed[count:])
	return fenceSpec{delimiter: first, length: count, info: info}, true
}

func parseFencedCodeBlock(lines []string, start int, fence fenceSpec) (markdownCodeBlock, int) {
	var content []string
	i := start + 1
	for i < len(lines) {
		line := lines[i]
		lineIndent := leadingSpaces(line)
		trimmed := strings.TrimLeft(line, " \t")
		if lineIndent <= fencedCodeIndentLimit {
			if closing, ok := detectFence(trimmed); ok && closing.delimiter == fence.delimiter && closing.length >= fence.length {
				return markdownCodeBlock{
					info:     fence.info,
					lines:    content,
					fenced:   true,
					indented: false,
				}, i + 1
			}
		}
		content = append(content, line)
		i++
	}
	return markdownCodeBlock{
		info:     fence.info,
		lines:    content,
		fenced:   true,
		indented: false,
	}, i
}

func parseIndentedCodeBlock(lines []string, start int) (markdownCodeBlock, int) {
	var content []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			content = append(content, "")
			i++
			continue
		}
		if leadingSpaces(line) < 4 {
			break
		}
		content = append(content, line[4:])
		i++
	}
	return markdownCodeBlock{
		info:     "",
		lines:    content,
		fenced:   false,
		indented: true,
	}, i
}

type listMarker struct {
	ordered bool
	// number is non-zero for ordered lists, otherwise holds inferred start.
	number    int
	markerLen int
	indent    int
	content   string
	raw       string
}

func parseList(lines []string, start int, depth int) (markdownList, int, bool) {
	marker, ok := parseListMarker(lines[start])
	if !ok {
		return markdownList{}, start, false
	}

	baseIndent := marker.indent
	list := markdownList{
		ordered: marker.ordered,
		start:   marker.number,
	}
	if !marker.ordered {
		list.start = 1
	}

	i := start
	for i < len(lines) {
		m, ok := parseListMarker(lines[i])
		if !ok || m.indent != baseIndent || m.ordered != list.ordered {
			break
		}
		if m.number == 0 {
			m.number = list.start + len(list.items)
		}

		itemLines := []string{m.content}
		contentIndent := baseIndent + m.markerLen + 1
		codeIndent := contentIndent + 4
		inFence := false
		var fenceDelimiter rune
		var fenceLength int
		i++
		for i < len(lines) {
			line := lines[i]
			if isBlankLine(line) {
				itemLines = append(itemLines, "")
				i++
				continue
			}

			if !inFence {
				if nextMarker, ok := parseListMarker(line); ok && nextMarker.indent == baseIndent {
					break
				}
			}

			if leadingSpaces(line) < contentIndent {
				break
			}

			content := line[contentIndent:]
			contentLeading := leadingSpaces(content)
			trimmedContent := strings.TrimLeft(content, " \t")

			if inFence {
				itemLines = append(itemLines, trimmedContent)
				if contentLeading <= fencedCodeIndentLimit {
					if count := countRepeatRune(trimmedContent, fenceDelimiter); count >= fenceLength {
						inFence = false
					}
				}
				i++
				continue
			}

			if contentLeading <= fencedCodeIndentLimit {
				if fence, ok := detectFence(trimmedContent); ok {
					inFence = true
					fenceDelimiter = fence.delimiter
					fenceLength = fence.length
					itemLines = append(itemLines, trimmedContent)
					i++
					continue
				}
			}

			if leadingSpaces(line) >= codeIndent {
				itemLines = append(itemLines, trimmedContent)
				i++
				continue
			}

			startCol := contentIndent
			if startCol > len(line) {
				startCol = len(line)
			}
			itemLines = append(itemLines, line[startCol:])
			i++
		}

		itemBlocks, _ := parseBlocksWithDepth(itemLines, 0, depth+1)
		list.items = append(list.items, markdownListItem{
			blocks: itemBlocks,
		})
	}

	return list, i, true
}

func parseListMarker(line string) (listMarker, bool) {
	if isBlankLine(line) {
		return listMarker{}, false
	}
	indent := leadingSpaces(line)
	trimmed := line[indent:]
	if trimmed == "" {
		return listMarker{}, false
	}

	if isBullet(trimmed[0]) {
		if len(trimmed) < 2 || !isSpaceOrTab(rune(trimmed[1])) {
			return listMarker{}, false
		}
		content := strings.TrimLeft(trimmed[2:], " \t")
		return listMarker{
			ordered:   false,
			number:    0,
			markerLen: 1,
			indent:    indent,
			content:   content,
			raw:       line,
		}, true
	}

	if unicode.IsDigit(rune(trimmed[0])) {
		j := 0
		for j < len(trimmed) && unicode.IsDigit(rune(trimmed[j])) {
			j++
		}
		if j == 0 || j >= len(trimmed) {
			return listMarker{}, false
		}
		if trimmed[j] != '.' && trimmed[j] != ')' {
			return listMarker{}, false
		}
		if j+1 >= len(trimmed) || !isSpaceOrTab(rune(trimmed[j+1])) {
			return listMarker{}, false
		}
		num, _ := strconv.Atoi(trimmed[:j])
		content := strings.TrimLeft(trimmed[j+2:], " \t")
		return listMarker{
			ordered:   true,
			number:    num,
			markerLen: j + 1,
			indent:    indent,
			content:   content,
			raw:       line,
		}, true
	}

	return listMarker{}, false
}

func parseHeading(trimmed string) (int, string, bool) {
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
		level++
	}
	if level == 0 {
		return 0, "", false
	}
	content := strings.TrimSpace(trimmed[level:])
	return level, content, true
}

func parseSetextHeading(lines []string, index int) (int, bool) {
	if index+1 >= len(lines) {
		return 0, false
	}
	text := lines[index]
	if isBlankLine(text) {
		return 0, false
	}
	indicator := strings.TrimSpace(lines[index+1])
	if indicator == "" {
		return 0, false
	}
	level := 0
	if allRunes(indicator, '=') {
		level = 1
	} else if allRunes(indicator, '-') {
		level = 2
	}
	if level == 0 {
		return 0, false
	}
	return level, true
}

func startsBlock(lines []string, index int) bool {
	if index >= len(lines) {
		return false
	}
	trimmed := strings.TrimLeft(lines[index], " \t")
	if trimmed == "" {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return true
	case strings.HasPrefix(trimmed, ">"):
		return true
	case isHorizontalRule(trimmed):
		return true
	}
	if _, ok := detectFence(trimmed); ok {
		return true
	}
	if _, ok := parseListMarker(trimmed); ok {
		return true
	}
	if looksLikeTableStart(lines, index) {
		return true
	}
	return false
}

func isHorizontalRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	if strings.ContainsAny(trimmed, "0123456789") {
		return false
	}
	clean := strings.ReplaceAll(trimmed, " ", "")
	clean = strings.ReplaceAll(clean, "\t", "")
	if len(clean) < 3 {
		return false
	}
	allSame := func(r rune) bool {
		for _, ch := range clean {
			if ch != r {
				return false
			}
		}
		return true
	}
	return allSame('-') || allSame('*') || allSame('_')
}

func joinParagraphLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	hardPrev := false
	for idx, line := range lines {
		content, hard := normalizeParagraphLine(line)
		if idx > 0 {
			if hardPrev {
				b.WriteRune('\n')
			} else {
				b.WriteRune(' ')
			}
		}
		b.WriteString(content)
		hardPrev = hard
	}
	return b.String()
}

func normalizeParagraphLine(line string) (string, bool) {
	raw := strings.TrimRight(line, "\t")
	trimmed := strings.TrimRight(raw, " ")
	trailingSpaces := len(raw) - len(trimmed)
	hardBreak := trailingSpaces >= 2
	content := trimmed

	if strings.HasSuffix(content, "\\") {
		backslashes := trailingBackslashes(content)
		if backslashes%2 == 1 {
			hardBreak = true
			content = content[:len(content)-1]
		}
	}
	if hardBreak {
		content = strings.TrimRight(content, " ")
	}
	return content, hardBreak
}

func trailingBackslashes(s string) int {
	count := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count
}

func leadingSpaces(line string) int {
	count := 0
	for _, ch := range line {
		switch ch {
		case ' ', '\t':
			count++
		default:
			return count
		}
	}
	return count
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func isBullet(ch byte) bool {
	return ch == '-' || ch == '+' || ch == '*'
}

func isSpaceOrTab(r rune) bool {
	return r == ' ' || r == '\t'
}

func allRunes(s string, target rune) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != target {
			return false
		}
	}
	return true
}

func countRepeatRune(text string, target rune) int {
	n := 0
	for _, r := range text {
		if r != target {
			break
		}
		n++
	}
	return n
}
