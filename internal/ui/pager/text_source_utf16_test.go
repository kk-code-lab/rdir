package pager

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"golang.org/x/text/encoding/unicode"
)

func TestTextPagerSource_UTF16StreamingLE(t *testing.T) {
	runUTF16StreamingTest(t, fsutil.EncodingUTF16LE)
}

func TestTextPagerSource_UTF16StreamingBE(t *testing.T) {
	runUTF16StreamingTest(t, fsutil.EncodingUTF16BE)
}

// Additional regression: CRLF handling in UTF-16 should not leave extra \r.
func TestTextPagerSource_UTF16_CRLF(t *testing.T) {
	enc := fsutil.EncodingUTF16LE
	text := "first line\r\nsecond line\r\nthird"

	encoded, err := unicode.UTF16(endian(enc), unicode.ExpectBOM).NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "crlf.txt")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	content, err := fsutil.ReadFileHead(path, statepkg.PreviewByteLimitForTest())
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	preview := &statepkg.PreviewData{}
	ctx := statepkg.PreviewData{
		Name: path,
	}
	_ = ctx

	preview.TextEncoding = enc
	lines, meta, chars, rem := statepkg.BuildUTF16PreviewForTest(content, enc, false)
	preview.TextLines = lines
	preview.TextLineMeta = meta
	preview.TextCharCount = chars
	preview.TextTruncated = len(rem) > 0
	preview.TextBytesRead = int64(len(content))
	preview.TextRemainder = rem

	src, err := newTextPagerSource(path, preview)
	if err != nil {
		t.Fatalf("newTextPagerSource: %v", err)
	}
	if err := src.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}

	expected := []string{"first line", "second line", "third"}
	if src.LineCount() != len(expected) {
		t.Fatalf("line count %d want %d", src.LineCount(), len(expected))
	}
	for i, want := range expected {
		if got := src.Line(i); got != want {
			t.Fatalf("line %d = %q want %q", i, got, want)
		}
	}
}

func TestTextPagerSource_UTF16_CRLF_BE(t *testing.T) {
	enc := fsutil.EncodingUTF16BE
	text := "uno\r\ndos\r\ntres"

	encoded, err := unicode.UTF16(endian(enc), unicode.ExpectBOM).NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "crlf_be.txt")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	content, err := fsutil.ReadFileHead(path, statepkg.PreviewByteLimitForTest())
	if err != nil {
		t.Fatalf("read head: %v", err)
	}

	linesHead, meta, chars, rem := statepkg.BuildUTF16PreviewForTest(content, enc, false)
	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		TextEncoding:  enc,
		TextLines:     linesHead,
		TextLineMeta:  meta,
		TextCharCount: chars,
		TextTruncated: len(rem) > 0,
		TextBytesRead: int64(len(content)),
		TextRemainder: rem,
	}

	src, err := newTextPagerSource(path, preview)
	if err != nil {
		t.Fatalf("newTextPagerSource: %v", err)
	}
	if err := src.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}

	wantLines := []string{"uno", "dos", "tres"}
	if src.LineCount() != len(wantLines) {
		t.Fatalf("line count %d want %d", src.LineCount(), len(wantLines))
	}
	for i, want := range wantLines {
		if got := src.Line(i); got != want {
			t.Fatalf("line %d = %q want %q", i, got, want)
		}
	}
}

// Ensure limit flag is raised on real-sized file without mutating globals:
// encode enough lines to exceed default searchMaxLines (20k) in UTF-16 BE.
func TestTextPagerSource_UTF16_LimitDefault(t *testing.T) {
	enc := fsutil.EncodingUTF16BE

	lines := make([]string, 0, 21000)
	for i := 0; i < 21050; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	text := strings.Join(lines, "\n")
	encoded, err := unicode.UTF16(endian(enc), unicode.ExpectBOM).NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "limit.txt")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	head, err := fsutil.ReadFileHead(path, statepkg.PreviewByteLimitForTest())
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	linesHead, meta, chars, rem := statepkg.BuildUTF16PreviewForTest(head, enc, true)
	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		Size:          int64(len(encoded)),
		TextEncoding:  enc,
		TextLines:     linesHead,
		TextLineMeta:  meta,
		TextCharCount: chars,
		TextTruncated: true,
		TextBytesRead: int64(len(head)),
		TextRemainder: rem,
	}

	state := &statepkg.AppState{
		CurrentPath:        tmp,
		PreviewData:        preview,
		ClipboardAvailable: true,
	}

	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.executeSearch("line")
	if !pager.searchLimited {
		t.Fatalf("expected searchLimited to be true on large file")
	}
	if len(pager.searchHits) == 0 {
		t.Fatalf("expected some hits")
	}
	// Should not load whole file implicitly; limiter triggers without copy-all.
	if pager.rawTextSource != nil && pager.rawTextSource.FullyLoaded() {
		t.Fatalf("streamer should not load full file during limited search")
	}
}

func runUTF16StreamingTest(t *testing.T, enc fsutil.UnicodeEncoding) {
	t.Helper()

	linesFull := []string{"alpha", "beta", strings.Repeat("gamma", 20), "delta", "OmEgA"}
	text := strings.Join(linesFull, "\n")

	encoder := unicode.UTF16(endian(enc), unicode.ExpectBOM).NewEncoder()
	encoded, err := encoder.Bytes([]byte(text))
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "utf16.txt")
	if err := os.WriteFile(filePath, encoded, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	headLen := len(encoded) / 2
	head := append([]byte(nil), encoded[:headLen]...)
	headLines, headMeta, headRemainder, headCharCount := previewFromUTF16Head(t, head, enc)

	preview := &statepkg.PreviewData{
		Name:                     filepath.Base(filePath),
		Size:                     int64(len(encoded) * 2),
		TextEncoding:             enc,
		TextLines:                headLines,
		TextLineMeta:             headMeta,
		TextCharCount:            headCharCount,
		TextTruncated:            true,
		TextBytesRead:            int64(headLen),
		TextRemainder:            headRemainder,
		HiddenFormattingDetected: false,
	}

	source, err := newTextPagerSource(filePath, preview)
	if err != nil {
		t.Fatalf("newTextPagerSource: %v", err)
	}
	source.chunkSize = 32
	if err := source.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}

	if got, want := source.LineCount(), len(linesFull); got != want {
		t.Fatalf("LineCount got %d want %d", got, want)
	}
	for i, want := range linesFull {
		got := source.Line(i)
		if got != want {
			t.Fatalf("line %d mismatch: got %q want %q", i, got, want)
		}
	}

	// Ensure cached char count matches full content.
	expectedChars := 0
	for _, line := range linesFull {
		expectedChars += utf8RuneCount(line)
	}
	if got := source.CharCount(); got != expectedChars {
		t.Fatalf("CharCount got %d want %d", got, expectedChars)
	}
	if !source.FullyLoaded() {
		t.Fatalf("source should be fully loaded")
	}

	state := &statepkg.AppState{
		CurrentPath:        tmpDir,
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	copied := ""
	pager.clipboardFunc = func(s string) error {
		copied = s
		return nil
	}

	// Case-insensitive search should still find the tail token.
	// Search for tail token to ensure streaming covered beyond head buffer.
	pager.executeSearch("omega")
	if len(pager.searchHits) != 1 {
		t.Fatalf("searchHits len=%d, want 1", len(pager.searchHits))
	}
	if pager.searchHits[0].line != len(linesFull)-1 {
		t.Fatalf("search hit line=%d, want %d", pager.searchHits[0].line, len(linesFull)-1)
	}

	// Enable wrap and search within a long line to ensure highlights survive wrapping math.
	pager.wrapEnabled = true
	pager.width = 5
	pager.executeSearch("gamma")
	if len(pager.searchHits) == 0 {
		t.Fatalf("expected gamma hits in long line")
	}
	longLineIdx := 2
	pager.ensureRowMetrics()
	if span := pager.rowSpanForIndex(longLineIdx); span < 2 {
		t.Fatalf("expected wrapped rows for long line, got %d", span)
	}
	spans, focus := pager.visibleHighlights(longLineIdx, 0, pager.width)
	if len(spans) == 0 {
		t.Fatalf("expected visible highlights on wrapped line")
	}
	if focus == nil {
		t.Fatalf("expected focused span on wrapped line")
	}

	if _, _, err := pager.copyAllToClipboard(); err != nil {
		t.Fatalf("copyAllToClipboard: %v", err)
	}
	wantAll := strings.Join(linesFull, "\n")
	if copied != wantAll {
		t.Fatalf("clipboard content mismatch:\nwant %q\ngot  %q", wantAll, copied)
	}

	// Limit search reads and hits to exercise truncation flags by temporarily
	// shadowing the constants via local caching and a wrapper.
	oldLines := searchMaxLines
	oldHits := searchMaxHits
	searchMaxLines = 2
	searchMaxHits = 1
	defer func() {
		searchMaxLines = oldLines
		searchMaxHits = oldHits
	}()

	pager.executeSearch("a")
	if !pager.searchLimited {
		t.Fatalf("expected searchLimited with reduced limits")
	}
	if len(pager.searchHits) != 1 {
		t.Fatalf("expected 1 hit under limit, got %d", len(pager.searchHits))
	}
}

func endian(enc fsutil.UnicodeEncoding) unicode.Endianness {
	if enc == fsutil.EncodingUTF16BE {
		return unicode.BigEndian
	}
	return unicode.LittleEndian
}

func previewFromUTF16Head(t *testing.T, head []byte, enc fsutil.UnicodeEncoding) ([]string, []statepkg.TextLineMetadata, []byte, int) {
	t.Helper()

	offset := int64(0)
	data := head
	if len(data) >= 2 {
		offset = 2
		data = data[2:]
	}

	lines := make([]string, 0)
	meta := make([]statepkg.TextLineMetadata, 0)
	charCount := 0
	lineStart := 0

	for lineStart+1 < len(data) {
		idx := -1
		for i := lineStart; i+1 < len(data); i += 2 {
			if isUTF16LF(data[i], data[i+1], enc) {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		lineBytes := data[lineStart:idx]
		if len(lineBytes) >= 2 && isUTF16CR(lineBytes[len(lineBytes)-2], lineBytes[len(lineBytes)-1], enc) {
			lineBytes = lineBytes[:len(lineBytes)-2]
		}
		text, runes, width := decodeUTF16Line(lineBytes, enc)
		meta = append(meta, statepkg.TextLineMetadata{
			Offset:       offset + int64(lineStart),
			Length:       len(lineBytes),
			RuneCount:    runes,
			DisplayWidth: width,
		})
		lines = append(lines, text)
		charCount += runes
		lineStart = idx + 2
	}

	remainder := append([]byte(nil), data[lineStart:]...)
	return lines, meta, remainder, charCount
}

func decodeUTF16Line(lineBytes []byte, enc fsutil.UnicodeEncoding) (string, int, int) {
	if len(lineBytes) == 0 {
		return "", 0, 0
	}
	decoder := unicode.UTF16(endian(enc), unicode.IgnoreBOM).NewDecoder()
	utf8Bytes, err := decoder.Bytes(lineBytes)
	if err != nil {
		utf8Bytes = lineBytes
	}
	text := string(utf8Bytes)
	expanded := textutil.ExpandTabs(text, textutil.DefaultTabWidth)
	runes := utf8RuneCount(expanded)
	width := textutil.DisplayWidth(expanded)
	return expanded, runes, width
}

func utf8RuneCount(s string) int {
	return len([]rune(s))
}
