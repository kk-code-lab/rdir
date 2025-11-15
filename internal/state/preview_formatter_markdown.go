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
	if preview.TextTruncated {
		preview.FormattedUnavailableReason = "formatted preview unavailable: truncated content"
		return
	}
	if ctx.info.Size() > formattedPreviewMaxBytes {
		preview.FormattedUnavailableReason = "formatted preview unavailable: file too large"
		return
	}
	if len(preview.TextLines) == 0 {
		preview.FormattedUnavailableReason = "formatted preview unavailable: empty content"
		return
	}

	segments := formatMarkdownSegments(preview.TextLines)
	formatted := formatMarkdownLines(preview.TextLines)

	preview.FormattedSegments = segments
	preview.FormattedSegmentLineMeta = textLineMetadataFromSegments(segments)
	preview.FormattedTextLines = formatted
	preview.FormattedTextLineMeta = textLineMetadataFromSegments(segments)
	preview.FormattedUnavailableReason = ""
}
