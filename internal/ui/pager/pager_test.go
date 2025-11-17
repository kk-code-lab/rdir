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

	written := buf.String()
	expected := "\x1b[?25h\x1b[?7h"
	if written != expected {
		t.Fatalf("expected %q, got %q", expected, written)
	}
}

func TestPreviewPagerToggleFormatSwitchesViews(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "data.json",
		TextLines: []string{
			`{"name":"demo","count":1}`,
		},
		FormattedTextLines: []string{
			"{",
			`  "name": "demo",`,
			`  "count": 1`,
			"}",
		},
	}
	state := &statepkg.AppState{
		PreviewData: preview,
		CurrentPath: ".",
	}
	pager, err := NewPreviewPager(state, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	if !pager.showFormatted {
		t.Fatalf("expected pager to default to formatted view")
	}

	state.PreviewScrollOffset = 2
	state.PreviewWrapOffset = 1
	pager.handleKey(keyEvent{kind: keyToggleFormat})

	if pager.showFormatted {
		t.Fatalf("expected pager to switch to raw view after toggle")
	}
	if !state.PreviewPreferRaw {
		t.Fatalf("state should persist raw preference")
	}
	if state.PreviewScrollOffset != 0 || state.PreviewWrapOffset != 0 {
		t.Fatalf("expected scroll offsets reset after format toggle")
	}
	if len(pager.lines) == 0 || pager.lines[0] != preview.TextLines[0] {
		t.Fatalf("raw lines should be displayed after toggle")
	}

	pager.handleKey(keyEvent{kind: keyToggleFormat})
	if !pager.showFormatted {
		t.Fatalf("expected pager to return to formatted view on second toggle")
	}
	if state.PreviewPreferRaw {
		t.Fatalf("state should record formatted preference")
	}
}

func TestReadKeyEventShiftArrows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  keyKind
	}{
		{name: "shift-up", input: "\x1b[1;2A", want: keyShiftUp},
		{name: "shift-down", input: "\x1b[1;2B", want: keyShiftDown},
		{name: "ctrl-up", input: "\x1b[1;5A", want: keyUp},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &PreviewPager{
				reader: bufio.NewReader(strings.NewReader(tc.input)),
			}
			ev, err := p.readKeyEvent()
			if err != nil {
				t.Fatalf("readKeyEvent: %v", err)
			}
			if ev.kind != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, ev.kind)
			}
		})
	}
}

func TestReadKeyEventEdit(t *testing.T) {
	t.Parallel()
	p := &PreviewPager{reader: bufio.NewReader(strings.NewReader("e"))}
	ev, err := p.readKeyEvent()
	if err != nil {
		t.Fatalf("readKeyEvent: %v", err)
	}
	if ev.kind != keyOpenEditor {
		t.Fatalf("expected keyOpenEditor, got %v", ev.kind)
	}

	p = &PreviewPager{reader: bufio.NewReader(strings.NewReader("E"))}
	ev, err = p.readKeyEvent()
	if err != nil {
		t.Fatalf("readKeyEvent upper: %v", err)
	}
	if ev.kind != keyOpenEditor {
		t.Fatalf("expected keyOpenEditor for upper, got %v", ev.kind)
	}
}

func TestRestoreAfterEditorStreamingKeepsPosition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	var builder strings.Builder
	lineCount := 200
	for i := 0; i < lineCount; i++ {
		builder.WriteString(fmt.Sprintf("line %03d\n", i))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	lines := []string{"line 000", "line 001", "line 002", "line 003", "line 004"}
	metas := make([]statepkg.TextLineMetadata, len(lines))
	offset := int64(0)
	for i, l := range lines {
		metas[i] = statepkg.TextLineMetadata{
			Offset:       offset,
			Length:       len(l) + 1,
			RuneCount:    len(l) + 1,
			DisplayWidth: displayWidth(l + " \n"),
		}
		offset += int64(len(l) + 1)
	}

	preview := &statepkg.PreviewData{
		Name:          "big.txt",
		TextLines:     lines,
		TextLineMeta:  metas,
		TextBytesRead: offset,
		TextTruncated: true,
		TextEncoding:  fsutil.EncodingUnknown,
	}
	state := &statepkg.AppState{
		CurrentPath: dir,
		PreviewData: preview,
	}

	p := &PreviewPager{state: state, wrapEnabled: false, height: 20}
	p.restoreAfterEditor(150, 0)

	if got := p.state.PreviewScrollOffset; got != 150 {
		t.Fatalf("scroll offset lost: got %d want 150", got)
	}
	if got := p.lineCount(); got <= 150 {
		t.Fatalf("expected streamed lines to reach 151, got %d", got)
	}
}

func TestHandleKeyEditorStaysOpen(t *testing.T) {
	state := &statepkg.AppState{
		CurrentPath:     ".",
		PreviewData:     &statepkg.PreviewData{Name: "file.txt"},
		EditorAvailable: true,
	}
	p := &PreviewPager{state: state, editorCmd: []string{}}
	if done := p.handleKey(keyEvent{kind: keyOpenEditor}); done {
		t.Fatalf("pager should stay open after editor")
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
