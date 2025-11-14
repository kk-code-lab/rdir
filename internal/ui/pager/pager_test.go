package pager

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
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

func TestUpdateSizeFallsBackToOutputFd(t *testing.T) {
	original := termGetSize
	t.Cleanup(func() {
		termGetSize = original
	})

	var seen []int
	termGetSize = func(fd int) (int, int, error) {
		seen = append(seen, fd)
		if fd == 10 {
			return 0, 0, errors.New("no size")
		}
		if fd == 11 {
			return 80, 25, nil
		}
		return 0, 0, errors.New("unexpected fd")
	}

	input := os.NewFile(uintptr(10), "input-fd")
	output := os.NewFile(uintptr(11), "output-fd")
	t.Cleanup(func() {
		_ = input.Close()
		_ = output.Close()
	})

	p := &PreviewPager{
		input:      input,
		outputFile: output,
	}
	p.updateSize()

	if p.width != 80 || p.height != 25 {
		t.Fatalf("expected fallback size 80x25, got %dx%d", p.width, p.height)
	}
	if len(seen) < 2 {
		t.Fatalf("expected both descriptors to be attempted, got %v", seen)
	}
}

func TestTextPagerSourceStreamsLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	var builder strings.Builder
	totalLines := 300
	for i := 0; i < totalLines; i++ {
		fmt.Fprintf(&builder, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	preview := &statepkg.PreviewData{
		TextEncoding:  fsutil.EncodingUnknown,
		TextTruncated: true,
		TextBytesRead: 0,
	}
	source, err := newTextPagerSource(path, preview)
	if err != nil {
		t.Fatalf("newTextPagerSource: %v", err)
	}
	defer source.Close()

	if err := source.EnsureLine(120); err != nil {
		t.Fatalf("EnsureLine: %v", err)
	}
	if count := source.LineCount(); count <= 120 {
		t.Fatalf("expected to load more than 120 lines, got %d", count)
	}

	line := source.Line(123)
	if line != "line-123" {
		t.Fatalf("unexpected line 123 content: %q", line)
	}

	if err := source.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	if count := source.LineCount(); count != totalLines {
		t.Fatalf("expected %d total lines, got %d", totalLines, count)
	}
	if source.CharCount() <= 0 {
		t.Fatalf("expected char count to be tracked")
	}
}

func TestCleanupTerminalRestoresCursorAndWrap(t *testing.T) {
	var buf bytes.Buffer
	p := &PreviewPager{
		writer: bufio.NewWriter(&buf),
		output: &buf,
	}

	p.cleanupTerminal()

	if got := buf.String(); got != "\x1b[?25h\x1b[?7h" {
		t.Fatalf("expected cleanup to write cursor/wrap restore, got %q", got)
	}
}

func TestHeaderLinesSanitizeControlCharacters(t *testing.T) {
	state := &statepkg.AppState{
		CurrentPath: "/tmp",
		PreviewData: &statepkg.PreviewData{
			Name:     "bad\x1b[31m\nfile",
			Size:     42,
			Modified: time.Unix(0, 0),
			Mode:     0o644,
		},
	}
	p := &PreviewPager{
		state:    state,
		showInfo: true,
	}

	lines := p.headerLines()
	if len(lines) != 2 {
		t.Fatalf("expected two header lines, got %d", len(lines))
	}
	if lines[0] != "/tmp/bad?[31m file" {
		t.Fatalf("unexpected sanitized path: %q", lines[0])
	}
	if strings.Contains(lines[1], "\x1b") || strings.Contains(lines[1], "\n") {
		t.Fatalf("metadata line should not contain control characters: %q", lines[1])
	}
}

func TestDirEntryLineSanitizesNames(t *testing.T) {
	entry := statepkg.FileEntry{
		Name: "weird\x1b[0m\nname",
		Mode: 0o644,
	}
	line := dirEntryLine(entry)
	if strings.Contains(line, "\x1b") || strings.Contains(line, "\n") {
		t.Fatalf("directory entry line should not include control characters: %q", line)
	}
	if !strings.Contains(line, "weird?") {
		t.Fatalf("sanitized name should retain visible characters, got %q", line)
	}
}
