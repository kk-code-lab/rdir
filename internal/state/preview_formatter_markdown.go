package state

import (
	"path/filepath"
	"regexp"
	"strconv"
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
	mdStrong    = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	mdEmphasis  = regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
	mdCode      = regexp.MustCompile("`([^`]+)`")
	mdStrike    = regexp.MustCompile(`~~([^~]+)~~`)
	mdImageLink = regexp.MustCompile(`!\[([^\]]+)\]\(([^)]+)\)`)
	mdLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
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

	formatted := make([]string, len(preview.TextLines))
	for i, line := range preview.TextLines {
		formatted[i] = formatMarkdownLine(line)
	}
	preview.FormattedTextLines = formatted
	preview.FormattedTextLineMeta = textLineMetadataFromLines(formatted)
	preview.FormattedUnavailableReason = ""
}

func formatMarkdownLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	// Headings
	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
			level++
		}
		if level > 0 {
			rest := strings.TrimSpace(trimmed[level:])
			return strings.TrimSpace("H" + strconv.Itoa(level) + " " + rest)
		}
	}

	// Blockquote
	if strings.HasPrefix(trimmed, ">") {
		return strings.TrimSpace(strings.TrimLeft(trimmed[1:], " "))
	}

	return formatMarkdownInline(line)
}

func formatMarkdownInline(line string) string {
	line = mdImageLink.ReplaceAllString(line, "$1 ($2)")
	line = mdLink.ReplaceAllString(line, "$1 ($2)")
	line = mdCode.ReplaceAllString(line, "$1")
	line = mdStrong.ReplaceAllString(line, "$1$2")
	line = mdEmphasis.ReplaceAllString(line, "$1$2")
	line = mdStrike.ReplaceAllString(line, "$1")
	return line
}
