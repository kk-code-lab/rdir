package state

import (
	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

type textPreviewFormatter struct{}

func (textPreviewFormatter) CanHandle(ctx previewFormatContext) bool {
	return fsutil.IsTextFile(ctx.path, ctx.content)
}

func (textPreviewFormatter) Format(ctx previewFormatContext, preview *PreviewData) {
	preview.FormattedUnavailableReason = ""
	encoding := fsutil.DetectUnicodeEncoding(ctx.content)
	preview.TextEncoding = encoding
	preview.FormattedTextLines = nil
	preview.FormattedTextLineMeta = nil
	truncated := ctx.info.Size() > int64(len(ctx.content))
	if encoding == fsutil.EncodingUTF16LE || encoding == fsutil.EncodingUTF16BE {
		lines, meta, charCount, remainder := buildUTF16Preview(ctx.content, encoding, truncated)
		preview.TextLines = lines
		preview.TextLineMeta = meta
		preview.HiddenFormattingDetected = containsFormattingRunes(lines)
		preview.LineCount = len(lines)
		preview.TextCharCount = charCount
		preview.TextTruncated = truncated
		preview.TextBytesRead = int64(len(ctx.content))
		if len(remainder) > 0 {
			preview.TextRemainder = remainder
		} else {
			preview.TextRemainder = nil
		}
		preview.BinaryInfo = BinaryPreview{}
		return
	}

	lines, meta, charCount, remainder := buildTextPreview(ctx.content, truncated, encoding)
	preview.TextLines = lines
	preview.TextLineMeta = meta
	preview.HiddenFormattingDetected = containsFormattingRunes(lines)
	preview.LineCount = len(lines)
	preview.TextCharCount = charCount
	preview.TextTruncated = truncated
	preview.TextBytesRead = int64(len(ctx.content))
	if len(remainder) > 0 {
		preview.TextRemainder = remainder
	} else {
		preview.TextRemainder = nil
	}
	preview.BinaryInfo = BinaryPreview{}
}

func containsFormattingRunes(lines []string) bool {
	for _, line := range lines {
		if textutil.HasFormattingRunes(line) {
			return true
		}
	}
	return false
}
