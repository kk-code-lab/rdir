package state

type markdownRenderOptions struct {
	tableOpts tableRenderOptions
}

func defaultMarkdownRenderOptions() markdownRenderOptions {
	return markdownRenderOptions{
		tableOpts: tableRenderOptions{},
	}
}

func formatMarkdownLines(lines []string) []string {
	return formatMarkdownLinesWithOptions(lines, defaultMarkdownRenderOptions())
}

func formatMarkdownLinesWithOptions(lines []string, opts markdownRenderOptions) []string {
	doc := parseMarkdown(lines)
	return renderMarkdown(doc, opts)
}

func formatMarkdownSegments(lines []string) [][]StyledTextSegment {
	return formatMarkdownSegmentsWithOptions(lines, defaultMarkdownRenderOptions())
}

func formatMarkdownSegmentsWithOptions(lines []string, opts markdownRenderOptions) [][]StyledTextSegment {
	doc := parseMarkdown(lines)
	return renderMarkdownSegments(doc, opts)
}

// FormatMarkdownPreview builds styled segments and metadata for a markdown document,
// applying table rendering options such as width limits and line truncation.
func FormatMarkdownPreview(lines []string, tableWidth int, maxLinesPerCell int, wrap bool) ([][]StyledTextSegment, []TextLineMetadata) {
	opts := defaultMarkdownRenderOptions()
	if tableWidth > 1 {
		opts.tableOpts.MaxWidth = tableWidth - 1
	} else {
		opts.tableOpts.MaxWidth = tableWidth
	}
	if wrap {
		opts.tableOpts.MaxLinesPerCell = 0
	} else {
		opts.tableOpts.MaxLinesPerCell = maxLinesPerCell
	}
	opts.tableOpts.Ellipsis = "…"
	segments := formatMarkdownSegmentsWithOptions(lines, opts)
	meta := textLineMetadataFromSegments(segments)
	return segments, meta
}
