package state

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type markdownDocument struct {
	blocks []markdownBlock
}

type markdownBlock interface {
	blockType() markdownBlockType
}

type markdownBlockType int

const (
	blockParagraph markdownBlockType = iota
	blockHeading
	blockCode
	blockList
	blockBlockquote
	blockHorizontalRule
	blockTable
)

type markdownInlineType int

const (
	inlineText markdownInlineType = iota
	inlineEmphasis
	inlineStrong
	inlineStrike
	inlineCode
	inlineLink
	inlineImage
)

type markdownInline struct {
	kind        markdownInlineType
	literal     string
	children    []markdownInline
	destination string
}

type markdownHeading struct {
	level int
	text  []markdownInline
}

func (markdownHeading) blockType() markdownBlockType { return blockHeading }

type markdownParagraph struct {
	text []markdownInline
}

func (markdownParagraph) blockType() markdownBlockType { return blockParagraph }

type markdownCodeBlock struct {
	info     string
	lines    []string
	fenced   bool
	indented bool
}

func (markdownCodeBlock) blockType() markdownBlockType { return blockCode }

type markdownList struct {
	ordered bool
	start   int
	items   []markdownListItem
}

type markdownListItem struct {
	blocks []markdownBlock
}

func (markdownList) blockType() markdownBlockType { return blockList }

type markdownBlockquote struct {
	blocks []markdownBlock
}

func (markdownBlockquote) blockType() markdownBlockType { return blockBlockquote }

type markdownHorizontalRule struct{}

func (markdownHorizontalRule) blockType() markdownBlockType { return blockHorizontalRule }

type markdownTable struct {
	headers []string
	rows    [][]string
	align   []tableAlignment
	meta    []TextLineMetadata
}

func (markdownTable) blockType() markdownBlockType { return blockTable }

type tableAlignment int

const (
	alignDefault tableAlignment = iota
	alignLeft
	alignCenter
	alignRight
)

func formatMarkdownLines(lines []string) []string {
	doc := parseMarkdown(lines)
	return renderMarkdown(doc)
}

func formatMarkdownSegments(lines []string) [][]StyledTextSegment {
	doc := parseMarkdown(lines)
	return renderMarkdownSegments(doc)
}

func parseMarkdown(lines []string) markdownDocument {
	blocks, _ := parseBlocks(lines, 0)
	return markdownDocument{blocks: blocks}
}

func parseBlocks(lines []string, start int) ([]markdownBlock, int) {
	var blocks []markdownBlock
	i := start
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
			blockquote, next := parseBlockquote(lines, i)
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

		if list, next, ok := parseList(lines, i); ok {
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
		trimmed := strings.TrimLeft(line, " \t")
		if indent >= 4 || startsBlock(trimmed) {
			break
		}
		parts = append(parts, strings.TrimRight(line, " \t"))
		i++
	}

	text := strings.Join(parts, " ")
	return markdownParagraph{text: parseInline(text)}, i
}

func parseBlockquote(lines []string, start int) (markdownBlockquote, int) {
	var quoteLines []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
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

	children, _ := parseBlocks(quoteLines, 0)
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
		trimmed := strings.TrimLeft(line, " \t")
		if closing, ok := detectFence(trimmed); ok && closing.delimiter == fence.delimiter && closing.length >= fence.length {
			return markdownCodeBlock{
				info:     fence.info,
				lines:    content,
				fenced:   true,
				indented: false,
			}, i + 1
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
	ordered   bool
	number    int
	markerLen int
	indent    int
	content   string
	raw       string
}

func parseList(lines []string, start int) (markdownList, int, bool) {
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
		i++
		for i < len(lines) {
			line := lines[i]
			if isBlankLine(line) {
				itemLines = append(itemLines, "")
				i++
				continue
			}

			if nextMarker, ok := parseListMarker(line); ok && nextMarker.indent == baseIndent {
				break
			}

			if leadingSpaces(line) >= codeIndent {
				rel := line[contentIndent:]
				if strings.HasPrefix(strings.TrimLeft(rel, " \t"), "```") || strings.HasPrefix(strings.TrimLeft(rel, " \t"), "~~~") {
					break
				}
				itemLines = append(itemLines, strings.TrimLeft(line[contentIndent:], " \t"))
				i++
				continue
			}

			if leadingSpaces(line) >= contentIndent {
				startCol := contentIndent
				if startCol > len(line) {
					startCol = len(line)
				}
				itemLines = append(itemLines, line[startCol:])
				i++
				continue
			}
			break
		}

		itemBlocks, _ := parseBlocks(itemLines, 0)
		list.items = append(list.items, markdownListItem{blocks: itemBlocks})
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

func startsBlock(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	if strings.HasPrefix(trimmed, ">") {
		return true
	}
	if isHorizontalRule(trimmed) {
		return true
	}
	if _, ok := detectFence(trimmed); ok {
		return true
	}
	if _, ok := parseListMarker(trimmed); ok {
		return true
	}
	if looksLikeTableHeader(trimmed) {
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

	closeParen := findMatchingParen(runes[offset+1+endText+2:])
	if closeParen == -1 {
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

func findMatchingParen(runes []rune) int {
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
				return i
			}
			depth--
		}
		i++
	}
	return -1
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

func countRepeat(runes []rune, target rune) int {
	n := 0
	for n < len(runes) && runes[n] == target {
		n++
	}
	return n
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
		return []string{renderInlines(b.text)}
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
	tbl.meta = fancy.meta
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
		return [][]StyledTextSegment{renderInlineSegments(b.text, TextStylePlain)}
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

func parseTable(lines []string, start int) (markdownTable, int, bool) {
	if start+1 >= len(lines) {
		return markdownTable{}, start, false
	}
	header := strings.TrimSpace(lines[start])
	separator := strings.TrimSpace(lines[start+1])
	if !looksLikeTableHeader(header) || !looksLikeTableSeparator(separator) {
		return markdownTable{}, start, false
	}

	headers := splitTableRow(header)
	separators := splitTableRow(separator)
	if len(headers) == 0 || len(headers) != len(separators) {
		return markdownTable{}, start, false
	}
	align := parseTableAlignment(separators)

	var rows [][]string
	i := start + 2
	for i < len(lines) {
		line := lines[i]
		if isBlankLine(line) {
			break
		}
		if !strings.Contains(line, "|") {
			break
		}
		cells := splitTableRow(line)
		if len(cells) != len(headers) {
			break
		}
		rows = append(rows, cells)
		i++
	}

	return markdownTable{
		headers: headers,
		rows:    rows,
		align:   align,
	}, i, true
}

func looksLikeTableHeader(line string) bool {
	return strings.Contains(line, "|") && strings.Count(line, "|") >= 1
}

func looksLikeTableSeparator(line string) bool {
	parts := splitTableRow(line)
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		if strings.IndexFunc(part, func(r rune) bool { return r != '-' && r != ':' }) != -1 {
			return false
		}
	}
	return true
}

func parseTableAlignment(parts []string) []tableAlignment {
	align := make([]tableAlignment, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		left := strings.HasPrefix(part, ":")
		right := strings.HasSuffix(part, ":")
		switch {
		case left && right:
			align[i] = alignCenter
		case right:
			align[i] = alignRight
		case left:
			align[i] = alignLeft
		default:
			align[i] = alignDefault
		}
	}
	return align
}

func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func renderTable(tbl markdownTable) []string {
	if len(tbl.headers) == 0 {
		return nil
	}
	fancy := buildFormattedTable(tbl.headers, tbl.rows, tbl.align)
	return fancy.rows
}
