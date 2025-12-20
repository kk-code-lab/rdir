package state

import (
	"path/filepath"
	"strings"
)

type markdownPreviewFormatter struct{}

var (
	markdownExts = map[string]struct{}{
		".md":       {},
		".markdown": {},
		".mdown":    {},
		".mkd":      {},
		".mkdown":   {},
		".mdwn":     {},
	}
)

func (markdownPreviewFormatter) CanHandle(ctx previewFormatContext) bool {
	if ctx.info == nil || ctx.info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(ctx.path))
	_, ok := markdownExts[ext]
	return ok
}

func (markdownPreviewFormatter) Format(ctx previewFormatContext, preview *PreviewData) {
	textPreviewFormatter{}.Format(ctx, preview)
	if preview == nil {
		return
	}
	preview.FormattedKind = "markdown"
	preview.MarkdownFrontmatter = nil
	preview.MarkdownFrontmatterRaw = ""
	if preview.TextTruncated {
		preview.FormattedUnavailableReason = "no preview available: truncated content"
		return
	}
	if ctx.info.Size() > formattedPreviewMaxBytes {
		preview.FormattedUnavailableReason = "no preview available: file too large"
		return
	}
	if len(preview.TextLines) == 0 {
		preview.FormattedUnavailableReason = "no preview available: empty content"
		return
	}

	opts := defaultMarkdownRenderOptions()
	lines := preview.TextLines
	if meta, raw, body, ok := splitMarkdownFrontmatter(lines); ok {
		preview.MarkdownFrontmatter = meta
		preview.MarkdownFrontmatterRaw = raw
		lines = body
	}
	doc := parseMarkdown(lines)
	segments := renderMarkdownSegmentsWithDoc(doc, opts)
	formatted := renderMarkdownLinesWithDoc(doc, opts)

	preview.markdownDoc = &doc
	preview.FormattedSegments = segments
	preview.FormattedSegmentLineMeta = textLineMetadataFromSegments(segments)
	preview.FormattedTextLines = formatted
	preview.FormattedTextLineMeta = textLineMetadataFromSegments(segments)
	preview.FormattedUnavailableReason = ""
}
