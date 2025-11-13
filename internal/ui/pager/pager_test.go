package pager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestTrimWrappedPrefix(t *testing.T) {
	p := &PreviewPager{
		wrapEnabled: true,
		width:       4,
	}

	got := p.trimWrappedPrefix("abcdefgh", 1)
	if got != "efgh" {
		t.Fatalf("trimWrappedPrefix mismatch, got %q want %q", got, "efgh")
	}

	// Wide runes should be treated as a single column unit.
	got = p.trimWrappedPrefix("你好世界", 1)
	if got != "世界" {
		t.Fatalf("expected to trim two wide runes, got %q", got)
	}
}

func TestScrollRowsSingleLine(t *testing.T) {
	line := "abcdefghijklmnopqrstuvwxyz"
	state := &statepkg.AppState{}
	p := &PreviewPager{
		state:       state,
		wrapEnabled: true,
		width:       5,
		lines:       []string{line},
		lineWidths:  []int{displayWidth(line)},
	}

	p.scrollRows(len(p.lines), 3)
	if state.PreviewScrollOffset != 0 {
		t.Fatalf("expected line index 0, got %d", state.PreviewScrollOffset)
	}
	if state.PreviewWrapOffset != 3 {
		t.Fatalf("expected wrap offset 3, got %d", state.PreviewWrapOffset)
	}

	p.scrollRows(len(p.lines), 10)
	rows := p.rowSpanForIndex(0)
	if state.PreviewScrollOffset != 0 {
		t.Fatalf("expected to stay on single line, got index %d", state.PreviewScrollOffset)
	}
	if state.PreviewWrapOffset != rows-1 {
		t.Fatalf("expected wrap offset %d at bottom, got %d", rows-1, state.PreviewWrapOffset)
	}

	p.scrollRows(len(p.lines), -1)
	if state.PreviewWrapOffset != rows-2 {
		t.Fatalf("expected to move up one row, got wrap offset %d", state.PreviewWrapOffset)
	}
}

func TestBinaryPagerSourceReadsChunks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	dataSize := binaryPagerChunkSize + 32
	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte(i)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	source, err := newBinaryPagerSource(path, int64(len(data)))
	if err != nil {
		t.Fatalf("newBinaryPagerSource: %v", err)
	}
	defer source.Close()

	expectedLines := (len(data) + binaryPreviewLineWidth - 1) / binaryPreviewLineWidth
	if count := source.LineCount(); count != expectedLines {
		t.Fatalf("LineCount=%d want %d", count, expectedLines)
	}

	line := source.Line(0)
	if !strings.HasPrefix(line, "00000000") {
		t.Fatalf("unexpected first data line: %q", line)
	}

	secondChunkIdx := binaryPagerChunkSize / binaryPreviewLineWidth
	line = source.Line(secondChunkIdx)
	if !strings.HasPrefix(line, "00010000") {
		t.Fatalf("expected offset 0x10000, got %q", line)
	}
}
