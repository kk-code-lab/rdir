package state

import fsutil "github.com/kk-code-lab/rdir/internal/fs"

// BuildUTF16PreviewForTest is a test helper to access UTF-16 preview parsing
// without exporting the production function. It mirrors buildUTF16Preview.
func BuildUTF16PreviewForTest(content []byte, enc fsutil.UnicodeEncoding, truncated bool) ([]string, []TextLineMetadata, int, []byte) {
	return buildUTF16Preview(content, enc, truncated)
}
