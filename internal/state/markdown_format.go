package state

func formatMarkdownLines(lines []string) []string {
	doc := parseMarkdown(lines)
	return renderMarkdown(doc)
}

func formatMarkdownSegments(lines []string) [][]StyledTextSegment {
	doc := parseMarkdown(lines)
	return renderMarkdownSegments(doc)
}
