package state

type binaryPreviewFormatter struct{}

func (binaryPreviewFormatter) CanHandle(ctx previewFormatContext) bool {
	return true
}

func (binaryPreviewFormatter) Format(ctx previewFormatContext, preview *PreviewData) {
	preview.BinaryInfo = formatBinaryPreviewLines(ctx.content, ctx.info.Size())
	preview.LineCount = int((ctx.info.Size()+binaryPreviewLineWidth-1)/binaryPreviewLineWidth) + 1
	preview.TextLines = nil
	preview.TextLineMeta = nil
	preview.TextRemainder = nil
	preview.FormattedTextLines = nil
	preview.FormattedTextLineMeta = nil
}
