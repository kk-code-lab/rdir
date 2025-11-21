package state

// PreviewByteLimitForTest exposes previewByteLimit to tests in other packages.
func PreviewByteLimitForTest() int64 {
	return previewByteLimit
}
