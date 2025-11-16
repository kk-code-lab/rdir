package state

type markdownRenderOptions struct {
	tableOpts tableRenderOptions
}

func defaultMarkdownRenderOptions() markdownRenderOptions {
	return markdownRenderOptions{
		tableOpts: tableRenderOptions{},
	}
}

func renderMarkdownSegmentsWithDoc(doc markdownDocument, opts markdownRenderOptions) [][]StyledTextSegment {
	return renderMarkdownSegments(doc, opts)
}

func renderMarkdownLinesWithDoc(doc markdownDocument, opts markdownRenderOptions) []string {
	segments := renderMarkdownSegmentsWithDoc(doc, opts)
	return segmentsToTextLines(segments)
}

func formatMarkdownLines(lines []string) []string {
	return formatMarkdownLinesWithOptions(lines, defaultMarkdownRenderOptions())
}

func formatMarkdownLinesWithOptions(lines []string, opts markdownRenderOptions) []string {
	doc := parseMarkdown(lines)
	return renderMarkdownLinesWithDoc(doc, opts)
}

// FormatMarkdownPreview builds styled segments and metadata for a markdown document,
// applying table rendering options such as width limits and line truncation.
func FormatMarkdownPreview(lines []string, tableWidth int, maxLinesPerCell int, wrap bool) ([][]StyledTextSegment, []TextLineMetadata) {
	doc := parseMarkdown(lines)
	return formatMarkdownPreviewWithDoc(doc, tableWidth, maxLinesPerCell, wrap)
}

// FormatMarkdownPreviewFromData re-renders markdown preview content using a cached document when available.
func FormatMarkdownPreviewFromData(preview *PreviewData, tableWidth int, maxLinesPerCell int, wrap bool) ([][]StyledTextSegment, []TextLineMetadata) {
	if preview == nil {
		return nil, nil
	}
	if preview.markdownDoc != nil {
		return formatMarkdownPreviewWithDoc(*preview.markdownDoc, tableWidth, maxLinesPerCell, wrap)
	}
	return FormatMarkdownPreview(preview.TextLines, tableWidth, maxLinesPerCell, wrap)
}

func formatMarkdownPreviewWithDoc(doc markdownDocument, tableWidth int, maxLinesPerCell int, wrap bool) ([][]StyledTextSegment, []TextLineMetadata) {
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
	segments := renderMarkdownSegmentsWithDoc(doc, opts)
	meta := textLineMetadataFromSegments(segments)
	return segments, meta
}

func segmentsToTextLines(lines [][]StyledTextSegment) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = joinSegmentsText(line)
	}
	return out
}
