package state

import "os"

type previewFormatContext struct {
	path    string
	info    os.FileInfo
	content []byte
}

type previewFormatter interface {
	CanHandle(ctx previewFormatContext) bool
	Format(ctx previewFormatContext, preview *PreviewData)
}

var previewFormatters = []previewFormatter{
	markdownPreviewFormatter{},
	jsonPreviewFormatter{},
	textPreviewFormatter{},
	binaryPreviewFormatter{},
}
