package state

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

type jsonPreviewFormatter struct{}

func (jsonPreviewFormatter) CanHandle(ctx previewFormatContext) bool {
	if ctx.info == nil || ctx.info.IsDir() {
		return false
	}
	if ctx.info.Size() > int64(len(ctx.content)) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(ctx.path))
	if ext == ".json" {
		return true
	}
	trimmed := bytes.TrimLeft(ctx.content, " \t\r\n")
	if len(trimmed) == 0 {
		return false
	}
	first := trimmed[0]
	return first == '{' || first == '['
}

func (jsonPreviewFormatter) Format(ctx previewFormatContext, preview *PreviewData) {
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

	encoding := fsutil.DetectUnicodeEncoding(ctx.content)
	if encoding == fsutil.EncodingUTF16LE || encoding == fsutil.EncodingUTF16BE {
		preview.FormattedUnavailableReason = "formatted preview unavailable: unsupported encoding"
		return
	}

	source := ctx.content
	if encoding == fsutil.EncodingUTF8BOM {
		if len(source) >= 3 {
			source = source[3:]
		}
	}
	if len(source) == 0 {
		preview.FormattedUnavailableReason = "formatted preview unavailable: empty content"
		return
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, source, "", "  "); err != nil {
		preview.FormattedUnavailableReason = "formatted preview unavailable: invalid JSON"
		return
	}

	lines := strings.Split(buf.String(), "\n")
	expanded, _ := expandPreviewTextLines(lines)
	preview.FormattedTextLines = expanded
	preview.FormattedTextLineMeta = textLineMetadataFromLines(expanded)
	preview.FormattedUnavailableReason = ""
}
