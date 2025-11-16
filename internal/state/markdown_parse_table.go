package state

import "strings"

func parseTable(lines []string, start int) (markdownTable, int, bool) {
	if start+1 >= len(lines) {
		return markdownTable{}, start, false
	}
	header := strings.TrimSpace(lines[start])
	separator := strings.TrimSpace(lines[start+1])
	if !looksLikeTableHeader(header) || !looksLikeTableSeparator(separator) {
		return markdownTable{}, start, false
	}

	headerCells := splitTableRow(header)
	separators := splitTableRow(separator)
	if len(headerCells) == 0 || len(headerCells) != len(separators) {
		return markdownTable{}, start, false
	}
	align := parseTableAlignment(separators)

	headers := make([][]markdownInline, len(headerCells))
	for i, cell := range headerCells {
		headers[i] = parseInline(cell)
	}

	var rows [][][]markdownInline
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
		row := make([][]markdownInline, len(cells))
		for j, cell := range cells {
			row[j] = parseInline(cell)
		}
		rows = append(rows, row)
		i++
	}

	return markdownTable{
		headers: headers,
		rows:    rows,
		align:   align,
	}, i, true
}

func looksLikeTableStart(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	header := strings.TrimSpace(lines[index])
	separator := strings.TrimSpace(lines[index+1])
	if !looksLikeTableHeader(header) || !looksLikeTableSeparator(separator) {
		return false
	}
	headers := splitTableRow(header)
	separators := splitTableRow(separator)
	return len(headers) > 0 && len(headers) == len(separators)
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
	parts := splitPipes(line)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func splitPipes(line string) []string {
	var parts []string
	var buf []rune
	inCode := false
	backticks := 0
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '\\':
			if i+1 < len(runes) {
				buf = append(buf, runes[i+1])
				i++
				continue
			}
		case '`':
			if !inCode {
				backticks = countRepeat(runes[i:], '`')
				inCode = true
				buf = append(buf, runes[i:i+backticks]...)
				i += backticks - 1
				continue
			}
			if countRepeat(runes[i:], '`') == backticks {
				buf = append(buf, runes[i:i+backticks]...)
				i += backticks - 1
				inCode = false
				backticks = 0
				continue
			}
		case '|':
			if !inCode {
				parts = append(parts, string(buf))
				buf = buf[:0]
				continue
			}
		}
		buf = append(buf, r)
	}
	parts = append(parts, string(buf))
	return parts
}
