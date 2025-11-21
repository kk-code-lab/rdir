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
	pager, err := NewPreviewPager(state, nil, nil, nil)
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

func TestReadKeyEventCopy(t *testing.T) {
	t.Parallel()
	p := &PreviewPager{reader: bufio.NewReader(strings.NewReader("cC"))}

	ev, err := p.readKeyEvent()
	if err != nil {
		t.Fatalf("readKeyEvent: %v", err)
	}
	if ev.kind != keyCopyVisible {
		t.Fatalf("expected keyCopyVisible, got %v", ev.kind)
	}

	ev, err = p.readKeyEvent()
	if err != nil {
		t.Fatalf("readKeyEvent upper: %v", err)
	}
	if ev.kind != keyCopyAll {
		t.Fatalf("expected keyCopyAll, got %v", ev.kind)
	}
}

func TestCopyVisibleToClipboardCopiesViewport(t *testing.T) {
	t.Parallel()
	preview := &statepkg.PreviewData{
		Name: "file.txt",
		TextLines: []string{
			"first line",
			"second line",
			"third line",
			"fourth line",
		},
	}
	state := &statepkg.AppState{
		CurrentPath:         "/tmp",
		PreviewData:         preview,
		ClipboardAvailable:  true,
		PreviewScrollOffset: 0,
	}
	pager, err := NewPreviewPager(state, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.height = 5 // 1 header + 3 content + 1 status
	pager.width = 20 // no truncation
	var copied string
	pager.clipboardFunc = func(content string) error {
		copied = content
		return nil
	}

	if done := pager.handleKey(keyEvent{kind: keyCopyVisible}); done {
		t.Fatalf("copy action should not exit pager")
	}

	want := "first line\nsecond line\nthird line"
	if copied != want {
		t.Fatalf("copied visible content mismatch:\nwant=%q\ngot =%q", want, copied)
	}
	if state.LastYankTime.IsZero() {
		t.Fatalf("copy should record LastYankTime")
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
	t.Cleanup(func() { cleanupPagerSources(t, p) })
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
		CurrentPath: filepath.FromSlash("/tmp"),
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
	want := filepath.Join(filepath.FromSlash("/tmp"), "bad?[31m file")
	if lines[0] != want {
		t.Fatalf("unexpected sanitized path: %q (want %q)", lines[0], want)
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

func TestStatusLineBinaryOmitsWrapAndFormat(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:     "blob.bin",
		Size:     32,
		Mode:     0o644,
		Modified: time.Unix(0, 0),
		BinaryInfo: statepkg.BinaryPreview{
			Lines:      []string{"00000000"},
			TotalBytes: 32,
		},
		LineCount: 4,
	}
	state := &statepkg.AppState{PreviewData: preview}
	p := &PreviewPager{state: state, binaryMode: true}
	status := p.statusLine(preview.LineCount, 2, int(preview.BinaryInfo.TotalBytes))
	if strings.Contains(status, "wrap:") {
		t.Fatalf("binary status should not mention wrap, got %q", status)
	}
	if strings.Contains(status, "fmt:") {
		t.Fatalf("binary status should not mention fmt, got %q", status)
	}
	if !strings.Contains(status, "type:binary") {
		t.Fatalf("binary status should label type, got %q", status)
	}
	if !strings.Contains(status, "bytes") {
		t.Fatalf("binary status should show byte counts, got %q", status)
	}
	if !strings.Contains(status, "offset:") {
		t.Fatalf("binary status should show offset, got %q", status)
	}
	if !strings.Contains(status, "pos:") {
		t.Fatalf("binary status should show percentage, got %q", status)
	}
	if !strings.Contains(status, "? help") {
		t.Fatalf("help segment should advertise help toggle, got %q", status)
	}
}

func TestBinaryJumpSmallForward(t *testing.T) {
	t.Parallel()
	total := int64(128 * 1024)
	state := &statepkg.AppState{
		PreviewData: &statepkg.PreviewData{
			Name: "blob.bin",
			Size: total,
			BinaryInfo: statepkg.BinaryPreview{
				TotalBytes: total,
			},
		},
	}
	p := &PreviewPager{
		state:      state,
		binaryMode: true,
		binarySource: &binaryPagerSource{
			totalBytes:   total,
			bytesPerLine: binaryPreviewLineWidth,
		},
		height: 25,
		width:  80,
	}

	if done := p.handleKey(keyEvent{kind: keyJumpForwardSmall}); done {
		t.Fatalf("jump should not exit pager")
	}
	expected := binaryJumpSmallBytes / binaryPreviewLineWidth
	if p.state.PreviewScrollOffset != expected {
		t.Fatalf("expected offset line %d after jump, got %d", expected, p.state.PreviewScrollOffset)
	}
}

func TestBinaryJumpClampsAtEnd(t *testing.T) {
	t.Parallel()
	total := int64(10 * 1024)
	lines := int((total + int64(binaryPreviewLineWidth) - 1) / int64(binaryPreviewLineWidth))

	state := &statepkg.AppState{
		PreviewData: &statepkg.PreviewData{
			Name: "blob.bin",
			Size: total,
			BinaryInfo: statepkg.BinaryPreview{
				TotalBytes: total,
			},
		},
		PreviewScrollOffset: lines - 2,
	}
	p := &PreviewPager{
		state:      state,
		binaryMode: true,
		height:     25,
		width:      80,
		binarySource: &binaryPagerSource{
			totalBytes:   total,
			bytesPerLine: binaryPreviewLineWidth,
		},
	}

	if done := p.handleKey(keyEvent{kind: keyJumpForwardLarge}); done {
		t.Fatalf("jump should not exit pager")
	}
	visible := p.height - (len(p.headerLines()) + 1) - 1
	if visible < 1 {
		visible = 1
	}
	maxStart := lines - visible
	if maxStart < 0 {
		maxStart = 0
	}
	if p.state.PreviewScrollOffset != maxStart {
		t.Fatalf("expected clamp to last visible page start %d, got %d", maxStart, p.state.PreviewScrollOffset)
	}
}

func TestStatusLineTextShowsWrapAndFormat(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:               "data.json",
		TextLines:          []string{"{\"k\":1}"},
		FormattedTextLines: []string{"{", "  \"k\": 1", "}"},
		LineCount:          3,
		TextCharCount:      12,
	}
	state := &statepkg.AppState{PreviewData: preview}
	p := &PreviewPager{
		state:          state,
		wrapEnabled:    true,
		formattedLines: preview.FormattedTextLines,
		showFormatted:  true,
	}
	status := p.statusLine(len(preview.FormattedTextLines), 2, preview.TextCharCount)
	if !strings.Contains(status, "wrap:on") {
		t.Fatalf("text status should include wrap:on, got %q", status)
	}
	if !strings.Contains(status, "fmt:pretty") {
		t.Fatalf("text status should include fmt:pretty, got %q", status)
	}
	if !strings.Contains(status, "? help") {
		t.Fatalf("help segment should advertise help toggle, got %q", status)
	}
}

func TestHelpSegmentsCompactFooter(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "notes.txt",
		TextLines: []string{"hi"},
		LineCount: 1,
	}

	stateWithClipboard := &statepkg.AppState{
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(stateWithClipboard, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	segments := pager.helpSegments()
	if len(segments) < 2 || !containsSegment(segments, "? help") || !containsSegment(segments, "/ search") {
		t.Fatalf("footer help should advertise help and search, got %v", segments)
	}

	stateNoClipboard := &statepkg.AppState{
		PreviewData:        preview,
		ClipboardAvailable: false,
	}
	pagerNoClipboard, err := NewPreviewPager(stateNoClipboard, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	if !containsSegment(pagerNoClipboard.helpSegments(), "/ search") {
		t.Fatalf("footer help should still include help hint without clipboard")
	}
}

func TestExecuteSearchBuildsHighlights(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "notes.txt",
		TextLines: []string{
			"hello world",
			"second hello",
			"nomatch",
		},
		LineCount: 3,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.width = 40
	pager.executeSearch("hello")

	if got := len(pager.searchHits); got != 2 {
		t.Fatalf("expected 2 search hits, got %d", got)
	}
	spans, focus := pager.visibleHighlights(1, 0, 0)
	if len(spans) != 1 || spans[0].start != 7 || spans[0].end != 12 {
		t.Fatalf("unexpected highlight spans for line 1: %+v", spans)
	}
	if focus != nil {
		t.Fatalf("focus should not be on line 1")
	}
	highlighted := applySearchHighlights(pager.lineAt(1), spans, nil)
	if !strings.Contains(highlighted, searchHighlightOn) || !strings.Contains(highlighted, searchHighlightOff) {
		t.Fatalf("highlight codes should wrap match, got %q", highlighted)
	}
	if status := pager.searchStatusSegment(); status != "/hello 1/2" {
		t.Fatalf("search status segment mismatch, got %q", status)
	}
}

func TestSearchNavigationAdjustsWrapOffsets(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "wrap.txt",
		TextLines: []string{"aaaaabbbbbccccc"},
		LineCount: 1,
	}
	state := &statepkg.AppState{
		CurrentPath: "/tmp",
		PreviewData: preview,
		PreviewWrap: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.width = 5
	pager.height = 4
	pager.wrapEnabled = true

	pager.executeSearch("cc")
	if len(pager.searchHits) == 0 {
		t.Fatalf("expected search hit to be recorded")
	}

	pager.focusSearchHit(pager.searchCursor)
	if pager.state.PreviewScrollOffset != 0 {
		t.Fatalf("expected scroll offset to stay at first line, got %d", pager.state.PreviewScrollOffset)
	}
	if pager.state.PreviewWrapOffset != 1 {
		t.Fatalf("expected wrap offset to center hit, got %d", pager.state.PreviewWrapOffset)
	}
}

func TestSearchStatusPlaceholderWhenActive(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "notes.txt",
		TextLines: []string{"sample"},
		LineCount: 1,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.enterSearchMode()
	if seg := pager.searchStatusSegment(); seg != "/_" {
		t.Fatalf("expected placeholder search segment, got %q", seg)
	}

	pager.appendSearchRune('a')
	if seg := pager.searchStatusSegment(); seg != "/a_" {
		t.Fatalf("expected search segment to reflect input, got %q", seg)
	}
}

func TestSearchNavigationFinalizesPendingInput(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "demo.txt",
		TextLines: []string{
			"foo",
			"bar foo",
		},
		LineCount: 2,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.height = 8
	pager.width = 10
	pager.enterSearchMode()
	pager.searchInput = []rune("foo")
	pager.searchQuery = ""

	pager.handleSearchModeEvent(keyEvent{kind: keyDown})
	if pager.searchQuery != "foo" {
		t.Fatalf("expected search query to be finalized before navigation, got %q", pager.searchQuery)
	}
	if len(pager.searchHits) != 2 {
		t.Fatalf("expected search hits after navigation, got %d", len(pager.searchHits))
	}
	if pager.searchCursor != 1 {
		t.Fatalf("expected cursor to advance to second hit, got %d", pager.searchCursor)
	}
}

func TestSearchNavigationStartsAtFirstOffscreenHit(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "jump.txt",
		TextLines: []string{
			"lead 0",
			"lead 1",
			"lead 2",
			"lead 3",
			"lead 4",
			"first hit",
			"lead 6",
			"lead 7",
			"second hit",
			"lead 9",
		},
		LineCount: 10,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.height = 6
	pager.width = 20

	pager.executeSearch("hit")
	if len(pager.searchHits) != 2 {
		t.Fatalf("expected two hits, got %d", len(pager.searchHits))
	}
	if pager.hitVisible(pager.searchHits[0]) {
		t.Fatalf("first hit should be offscreen at start")
	}

	pager.moveSearchCursor(1) // first down should center the first hit, not skip it
	if pager.searchCursor != 0 {
		t.Fatalf("expected cursor to stay on first hit after initial jump, got %d", pager.searchCursor)
	}
	if !pager.hitVisible(pager.searchHits[0]) {
		t.Fatalf("first hit should become visible after jump")
	}

	pager.moveSearchCursor(1) // second down should advance
	if pager.searchCursor != 1 {
		t.Fatalf("expected cursor to advance to second hit, got %d", pager.searchCursor)
	}
}

func TestSearchEscapeClearsQueryAndHighlights(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "demo.txt",
		TextLines: []string{"foo bar"},
		LineCount: 1,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.enterSearchMode()
	pager.searchInput = []rune("foo")
	pager.finalizeSearchInput()
	if len(pager.searchHits) == 0 {
		t.Fatalf("expected hits after finalizing search input")
	}

	pager.handleSearchModeEvent(keyEvent{kind: keyEscape})

	if pager.searchMode {
		t.Fatalf("expected search mode to be disabled after escape")
	}
	if pager.searchQuery != "" {
		t.Fatalf("expected search query to clear, got %q", pager.searchQuery)
	}
	if len(pager.searchHits) != 0 || len(pager.searchHighlights) != 0 {
		t.Fatalf("expected search results to clear after escape")
	}
}

func TestSearchInitialCursorRespectsScrollPosition(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "demo.txt",
		TextLines: []string{
			"hit-one",
			"nope",
			"hit-two",
			"nope",
			"hit-three",
		},
		LineCount: 5,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	state.PreviewScrollOffset = 2
	pager.executeSearch("hit")
	if pager.searchCursor < 0 || pager.searchCursor >= len(pager.searchHits) {
		t.Fatalf("search cursor not set")
	}
	if pager.searchHits[pager.searchCursor].line != 2 {
		t.Fatalf("expected cursor to start at first hit at/after scroll, got line %d", pager.searchHits[pager.searchCursor].line)
	}

	state.PreviewScrollOffset = 10
	pager.executeSearch("hit")
	if pager.searchHits[pager.searchCursor].line != 0 {
		t.Fatalf("expected cursor to wrap to first hit when none after scroll, got line %d", pager.searchHits[pager.searchCursor].line)
	}
}

func TestSearchInitialCursorWrapAwareWithinLine(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "wrap-line",
		TextLines: []string{
			"hit---hit",
		},
		LineCount: 1,
	}
	state := &statepkg.AppState{
		CurrentPath: "/tmp",
		PreviewData: preview,
		PreviewWrap: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.width = 6
	pager.height = 10
	pager.wrapEnabled = true
	state.PreviewScrollOffset = 0
	state.PreviewWrapOffset = 1 // second wrapped row; should pick second hit

	pager.executeSearch("hit")
	if len(pager.searchHits) != 2 {
		t.Fatalf("expected two hits, got %d", len(pager.searchHits))
	}
	if pager.searchCursor < 0 || pager.searchCursor >= len(pager.searchHits) {
		t.Fatalf("search cursor not set")
	}
	if pager.searchHits[pager.searchCursor].span.start != 6 {
		t.Fatalf("expected cursor to choose hit in current wrapped row, got start %d", pager.searchHits[pager.searchCursor].span.start)
	}
}

func TestSearchSmartCase(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "case.txt",
		TextLines: []string{
			"Hit Upper",
			"hit lower",
		},
		LineCount: 2,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.executeSearch("hit")
	if len(pager.searchHits) != 2 {
		t.Fatalf("expected case-insensitive matches when query has no uppercase, got %d", len(pager.searchHits))
	}

	pager.executeSearch("Hit")
	if len(pager.searchHits) != 1 || pager.searchHits[0].line != 0 {
		t.Fatalf("expected case-sensitive match when query has uppercase")
	}
}

func TestSearchStaticMarksLimitedWhenCapHitInSingleLine(t *testing.T) {
	longLine := strings.Repeat("a", searchMaxHits+5)
	preview := &statepkg.PreviewData{
		Name:          "static.txt",
		TextLines:     []string{longLine},
		LineCount:     1,
		TextCharCount: len(longLine),
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.executeSearch("a")
	if len(pager.searchHits) != searchMaxHits {
		t.Fatalf("expected to collect max hits, got %d", len(pager.searchHits))
	}
	if !pager.searchLimited {
		t.Fatalf("search should report limited results after cap hit on single line")
	}
}

func TestSearchStreamingMarksLimitedWhenCapHit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream-cap.txt")
	longLine := strings.Repeat("a", searchMaxHits+5)
	if err := os.WriteFile(path, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	meta := statepkg.TextLineMetadata{
		Offset:       0,
		Length:       len(longLine),
		RuneCount:    len(longLine),
		DisplayWidth: displayWidth(longLine),
	}
	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		TextLines:     []string{longLine},
		TextLineMeta:  []statepkg.TextLineMetadata{meta},
		TextBytesRead: int64(len(longLine)),
		TextTruncated: true,
		TextEncoding:  fsutil.EncodingUnknown,
		LineCount:     1,
		TextCharCount: len(longLine),
	}
	state := &statepkg.AppState{CurrentPath: dir, PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager) })
	if pager.rawTextSource == nil {
		t.Fatalf("expected streaming source for truncated preview")
	}

	pager.executeSearch("a")
	if len(pager.searchHits) != searchMaxHits {
		t.Fatalf("expected to collect max hits, got %d", len(pager.searchHits))
	}
	if !pager.searchLimited {
		t.Fatalf("streaming search should report limited results after cap hit")
	}
}

func TestSearchStatusShowsCountsDuringActiveMode(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name: "notes.txt",
		TextLines: []string{
			"hello",
			"hello again",
		},
		LineCount: 2,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.executeSearch("hello")
	pager.enterSearchMode()
	pager.searchInput = []rune("hello")

	seg := pager.searchStatusSegment()
	if seg != "/hello_ 1/2" {
		t.Fatalf("expected active search to show counts, got %q", seg)
	}

	pager.searchInput = []rune("hel")
	seg = pager.searchStatusSegment()
	if seg != "/hel_" {
		t.Fatalf("expected counts to hide when input diverges, got %q", seg)
	}
}

func TestSearchModeConsumesCommandKeys(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "demo.txt",
		TextLines: []string{"wrap toggle should not fire"},
		LineCount: 1,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.wrapEnabled = false
	pager.handleKey(keyEvent{kind: keyStartSearch, ch: '/'})

	if !pager.searchMode {
		t.Fatalf("expected search mode after '/'")
	}

	pager.handleKey(keyEvent{kind: keyToggleWrap, ch: 'w'})
	if pager.wrapEnabled {
		t.Fatalf("wrap toggle should be ignored during search input")
	}
	pager.handleKey(keyEvent{kind: keyUp, ch: 'k'})
	pager.handleKey(keyEvent{kind: keySearchNext, ch: 'n'})

	if got := string(pager.searchInput); got != "wkn" {
		t.Fatalf("expected search input to collect typed keys, got %q", got)
	}
}

func TestFocusedHighlightUsesDistinctStyle(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "demo.txt",
		TextLines: []string{"foo foo"},
		LineCount: 1,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.executeSearch("foo")
	if len(pager.searchHits) != 2 {
		t.Fatalf("expected two hits, got %d", len(pager.searchHits))
	}

	spans, focus := pager.visibleHighlights(0, 0, 0)
	highlighted := applySearchHighlights(pager.lineAt(0), spans, focus)
	if !strings.Contains(highlighted, searchHighlightFocusOn) || !strings.Contains(highlighted, searchHighlightFocusOff) {
		t.Fatalf("focused span should use distinct style, got %q", highlighted)
	}
}

func TestHelpToggleClosesOverlayBeforeExit(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "notes.txt",
		TextLines: []string{"hi"},
		LineCount: 1,
	}
	state := &statepkg.AppState{PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	if done := pager.handleKey(keyEvent{kind: keyToggleHelp}); done {
		t.Fatalf("help toggle should not exit pager")
	}
	if !pager.showHelp {
		t.Fatalf("expected help overlay to be visible after '?'")
	}

	if done := pager.handleKey(keyEvent{kind: keyQuit}); done {
		t.Fatalf("quit should close help overlay first")
	}
	if pager.showHelp {
		t.Fatalf("help overlay should close on quit key")
	}

	if done := pager.handleKey(keyEvent{kind: keyQuit}); !done {
		t.Fatalf("pager should exit after quit when help overlay is closed")
	}
}

func TestReadKeyEventAcceptsHelpAlias(t *testing.T) {
	p := &PreviewPager{
		reader: bufio.NewReader(strings.NewReader("h")),
	}
	ev, err := p.readKeyEvent()
	if err != nil {
		t.Fatalf("readKeyEvent: %v", err)
	}
	if ev.kind != keyToggleHelp {
		t.Fatalf("expected help alias to map to toggle, got %v", ev.kind)
	}
}

func TestHelpOverlayReflectsContext(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:      "notes.txt",
		TextLines: []string{"hi"},
		LineCount: 1,
	}

	state := &statepkg.AppState{
		PreviewData:        preview,
		ClipboardAvailable: true,
		EditorAvailable:    true,
	}

	pager, err := NewPreviewPager(state, []string{"vim"}, nil, []string{"pbcopy"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.width = 80

	lines := pager.helpOverlayLines()
	if !containsLineWith(lines, "Copy visible lines") {
		t.Fatalf("clipboard hint should appear in help overlay, got %v", lines)
	}
	if !containsLineWith(lines, "Open in editor") {
		t.Fatalf("editor hint should appear in help overlay, got %v", lines)
	}

	pager.binaryMode = true
	pager.formattedLines = nil

	lines = pager.helpOverlayLines()
	if containsLineWith(lines, "wrap") {
		t.Fatalf("wrap hint should be hidden in binary mode help, got %v", lines)
	}
	if containsLineWith(lines, "formatted") {
		t.Fatalf("formatted hint should be hidden without formatted lines, got %v", lines)
	}

	pager.width = 40
	lines = pager.helpOverlayLines()
	if containsLineWith(lines, "---") {
		t.Fatalf("narrow help overlay should not include separators, got %v", lines)
	}

	pager.width = 80
	lines = pager.helpOverlayLines()
	if !containsLineWith(lines, "---") {
		t.Fatalf("wide help overlay should include separators, got %v", lines)
	}
}

func TestStatusLinePrioritizesStatusMessage(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:          "notes.txt",
		TextLines:     []string{"hi"},
		LineCount:     1,
		TextCharCount: 2,
	}
	state := &statepkg.AppState{PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.statusMessage = "copied view"
	line := pager.statusLine(1, 1, 2)
	if !strings.HasPrefix(line, "copied view") {
		t.Fatalf("status message should be first, got %q", line)
	}
}

func TestRecordCopyResultSetsStatusStyle(t *testing.T) {
	state := &statepkg.AppState{PreviewData: &statepkg.PreviewData{Name: "x.txt", TextLines: []string{"x"}}}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}

	pager.recordCopyResult(nil, "ok", "")
	if pager.statusStyle != statusSuccessStyle {
		t.Fatalf("expected success style, got %q", pager.statusStyle)
	}

	sentinel := errors.New("boom")
	pager.recordCopyResult(sentinel, "nope", "")
	if pager.statusStyle != statusErrorStyle {
		t.Fatalf("expected error style, got %q", pager.statusStyle)
	}

	pager.clearStatusMessage()
	if pager.statusStyle != "" || pager.statusMessage != "" {
		t.Fatalf("expected status state to clear, got style=%q msg=%q", pager.statusStyle, pager.statusMessage)
	}
}

func TestStatusLineMarksApproxChars(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:          "big.txt",
		TextLines:     []string{"data"},
		LineCount:     1,
		TextCharCount: 1024,
		TextTruncated: true,
	}
	state := &statepkg.AppState{PreviewData: preview}
	p := &PreviewPager{state: state}
	status := p.statusLine(preview.LineCount, 1, preview.TextCharCount)
	if !strings.Contains(status, "~1024 chars") {
		t.Fatalf("status should mark approximate char counts, got %q", status)
	}
}

func TestHeaderLinesIncludeTypeMetadata(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:                     "notes.txt",
		Size:                     2048,
		Mode:                     0o644,
		Modified:                 time.Unix(0, 0),
		LineCount:                42,
		TextCharCount:            100,
		TextEncoding:             fsutil.EncodingUTF8BOM,
		HiddenFormattingDetected: true,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	p := &PreviewPager{state: state, showInfo: true}
	lines := p.headerLines()
	if len(lines) < 2 {
		t.Fatalf("expected info line to be present, got %v", lines)
	}
	info := lines[1]
	if !strings.Contains(info, "type:text") {
		t.Fatalf("info line should include type metadata, got %q", info)
	}
	if !strings.Contains(info, "lines:42") {
		t.Fatalf("info line should include line count, got %q", info)
	}
	if !strings.Contains(info, "encoding:utf-8 bom") {
		t.Fatalf("info line should include encoding, got %q", info)
	}
}

func TestHeaderLinesIncludeApproxCharMetadata(t *testing.T) {
	preview := &statepkg.PreviewData{
		Name:          "huge.txt",
		Size:          4096,
		Mode:          0o644,
		Modified:      time.Unix(0, 0),
		LineCount:     10,
		TextCharCount: 5000,
		TextTruncated: true,
	}
	state := &statepkg.AppState{CurrentPath: "/tmp", PreviewData: preview}
	p := &PreviewPager{state: state, showInfo: true}
	lines := p.headerLines()
	if len(lines) < 2 {
		t.Fatalf("expected metadata line in header")
	}
	info := lines[1]
	if !strings.Contains(info, "chars:~5000") {
		t.Fatalf("info line should flag approximate char counts, got %q", info)
	}
}

func TestInfoSegmentsTrackStreamingLineCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	var builder strings.Builder
	totalLines := 40
	for i := 0; i < totalLines; i++ {
		fmt.Fprintf(&builder, "line %03d\n", i)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sample := []string{"line 000", "line 001", "line 002", "line 003", "line 004"}
	metas := make([]statepkg.TextLineMetadata, len(sample))
	offset := int64(0)
	for i, line := range sample {
		metas[i] = statepkg.TextLineMetadata{
			Offset:       offset,
			Length:       len(line) + 1,
			RuneCount:    len(line) + 1,
			DisplayWidth: displayWidth(line + " \n"),
		}
		offset += int64(len(line) + 1)
	}

	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		Size:          int64(len(builder.String())),
		Mode:          0o644,
		Modified:      time.Unix(0, 0),
		TextLines:     sample,
		TextLineMeta:  metas,
		TextTruncated: true,
		TextBytesRead: offset,
		TextEncoding:  fsutil.EncodingUnknown,
		LineCount:     len(sample),
		TextCharCount: lineCharCount(sample),
	}
	state := &statepkg.AppState{CurrentPath: dir, PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager) })

	segments := pager.detailInfoSegments(preview)
	if !containsSegment(segments, "lines:~5") {
		t.Fatalf("expected initial lines:~5, got %v", segments)
	}
	if !containsSegment(segments, "streaming from disk") {
		t.Fatalf("expected streaming indicator, got %v", segments)
	}

	if err := pager.rawTextSource.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	segments = pager.detailInfoSegments(preview)
	if !containsSegment(segments, fmt.Sprintf("lines:%d", totalLines)) {
		t.Fatalf("expected final lines:%d, got %v", totalLines, segments)
	}
	if containsSegment(segments, "streaming from disk") {
		t.Fatalf("streaming indicator should be cleared after load, got %v", segments)
	}
}

func containsSegment(segments []string, target string) bool {
	for _, seg := range segments {
		if seg == target {
			return true
		}
	}
	return false
}

func containsLineWith(lines []string, target string) bool {
	for _, line := range lines {
		if strings.Contains(line, target) {
			return true
		}
	}
	return false
}

func cleanupPagerSources(t *testing.T, p *PreviewPager) {
	t.Helper()
	if p == nil {
		return
	}
	if p.rawTextSource != nil {
		p.rawTextSource.Close()
	}
	if p.binarySource != nil {
		p.binarySource.Close()
	}
}

func TestPersistLoadedLinesUpdatesPreviewState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.txt")
	var builder strings.Builder
	totalLines := 25
	for i := 0; i < totalLines; i++ {
		fmt.Fprintf(&builder, "line %02d\n", i)
	}
	data := builder.String()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sample := []string{"line 00", "line 01"}
	metas := []statepkg.TextLineMetadata{
		{Offset: 0, Length: len("line 00\n"), RuneCount: len("line 00\n"), DisplayWidth: displayWidth("line 00 \n")},
		{Offset: int64(len("line 00\n")), Length: len("line 01\n"), RuneCount: len("line 01\n"), DisplayWidth: displayWidth("line 01 \n")},
	}
	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		Size:          int64(len(data)),
		Mode:          0o644,
		Modified:      time.Unix(0, 0),
		TextLines:     sample,
		TextLineMeta:  metas,
		TextTruncated: true,
		TextBytesRead: int64(len("line 00\n")) + int64(len("line 01\n")),
		TextEncoding:  fsutil.EncodingUnknown,
		LineCount:     len(sample),
		TextCharCount: lineCharCount(sample),
	}
	state := &statepkg.AppState{CurrentPath: dir, PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager) })
	if pager.rawTextSource == nil {
		t.Fatalf("expected streaming text source")
	}
	if err := pager.rawTextSource.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	pager.persistLoadedLines()
	if preview.TextTruncated {
		t.Fatalf("expected truncated flag cleared after full load")
	}
	if preview.TextCharCount != pager.rawTextSource.CharCount() {
		t.Fatalf("char count mismatch: got %d want %d", preview.TextCharCount, pager.rawTextSource.CharCount())
	}
	if preview.LineCount != pager.rawTextSource.LineCount() {
		t.Fatalf("line count mismatch: got %d want %d", preview.LineCount, pager.rawTextSource.LineCount())
	}

	// Simulate reopening the pager with the persisted preview data.
	pager2, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("reopen pager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager2) })
	if pager2.isLineCountApprox() {
		t.Fatalf("line counts should be exact after persistence")
	}
	if pager2.isCharCountApprox() {
		t.Fatalf("char counts should be exact after persistence")
	}
}

func TestPersistLoadedLinesAllowsSubsequentStreaming(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.txt")
	var builder strings.Builder
	totalLines := 20000
	for i := 0; i < totalLines; i++ {
		fmt.Fprintf(&builder, "line %03d\n", i)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	const sampleCount = 5
	sample := make([]string, sampleCount)
	metas := make([]statepkg.TextLineMetadata, sampleCount)
	var offset int64
	for i := 0; i < sampleCount; i++ {
		line := fmt.Sprintf("line %03d", i)
		sample[i] = line
		length := len(line) + 1
		metas[i] = statepkg.TextLineMetadata{Offset: offset, Length: length, RuneCount: length, DisplayWidth: displayWidth(line)}
		offset += int64(length)
	}
	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		Size:          int64(len(builder.String())),
		Mode:          0o644,
		Modified:      time.Unix(0, 0),
		TextLines:     sample,
		TextLineMeta:  metas,
		TextTruncated: true,
		TextBytesRead: offset,
		TextEncoding:  fsutil.EncodingUnknown,
		LineCount:     len(sample),
		TextCharCount: lineCharCount(sample),
	}
	state := &statepkg.AppState{CurrentPath: dir, PreviewData: preview}
	pager, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager) })
	if pager.rawTextSource == nil {
		t.Fatalf("expected streaming source for truncated file")
	}
	if err := pager.rawTextSource.EnsureLine(20); err != nil {
		t.Fatalf("EnsureLine: %v", err)
	}
	if pager.rawTextSource.FullyLoaded() {
		t.Fatalf("test requires partial load")
	}
	pager.persistLoadedLines()
	// Simulate closing pager; next run should resume streaming.
	pager2, err := NewPreviewPager(state, nil, nil, nil)
	if err != nil {
		t.Fatalf("reopen pager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager2) })
	if pager2.rawTextSource == nil {
		t.Fatalf("expected streaming source after persistence")
	}
	if err := pager2.rawTextSource.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll second session: %v", err)
	}
	if count := pager2.rawTextSource.LineCount(); count != totalLines {
		t.Fatalf("streaming stalled: got %d lines want %d", count, totalLines)
	}
}

func TestCopyAllStreamsFullFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "copy.txt")
	var builder strings.Builder
	totalLines := 6
	for i := 0; i < totalLines; i++ {
		fmt.Fprintf(&builder, "line %02d\n", i)
	}
	data := builder.String()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sample := []string{"line 00", "line 01"}
	metas := make([]statepkg.TextLineMetadata, len(sample))
	var offset int64
	for i, line := range sample {
		metas[i] = statepkg.TextLineMetadata{
			Offset:       offset,
			Length:       len(line) + 1,
			RuneCount:    len(line) + 1,
			DisplayWidth: displayWidth(line + " \n"),
		}
		offset += int64(len(line) + 1)
	}

	preview := &statepkg.PreviewData{
		Name:          filepath.Base(path),
		Size:          int64(len(data)),
		Mode:          0o644,
		Modified:      time.Unix(0, 0),
		TextLines:     sample,
		TextLineMeta:  metas,
		TextBytesRead: offset,
		TextTruncated: true,
		TextEncoding:  fsutil.EncodingUnknown,
		LineCount:     len(sample),
		TextCharCount: lineCharCount(sample),
	}
	state := &statepkg.AppState{
		CurrentPath:        dir,
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	t.Cleanup(func() { cleanupPagerSources(t, pager) })
	if pager.rawTextSource == nil {
		t.Fatalf("expected streaming source")
	}
	pager.width = 80
	pager.height = 10
	var copied string
	pager.clipboardFunc = func(content string) error {
		copied = content
		return nil
	}

	if done := pager.handleKey(keyEvent{kind: keyCopyAll}); done {
		t.Fatalf("copy action should not exit pager")
	}

	if !strings.Contains(copied, "line 05") {
		t.Fatalf("expected full file to be copied, got %q", copied)
	}
	if state.LastYankTime.IsZero() {
		t.Fatalf("copy should record LastYankTime")
	}
}

func TestCopyAllWarnsOnLargeFile(t *testing.T) {
	t.Parallel()
	preview := &statepkg.PreviewData{
		Name:          "big.txt",
		Size:          clipboardWarnBytes + 1,
		TextLines:     []string{"large"},
		LineCount:     1,
		TextCharCount: 5,
	}
	state := &statepkg.AppState{
		CurrentPath:        "/tmp",
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	var copied string
	pager.clipboardFunc = func(content string) error {
		copied = content
		return nil
	}

	msg, style, err := pager.copyAllToClipboard()
	if err != nil {
		t.Fatalf("copyAllToClipboard: %v", err)
	}
	pager.recordCopyResult(err, msg, style)

	if style != statusWarnStyle {
		t.Fatalf("expected warning style, got %q", style)
	}
	if pager.statusStyle != statusWarnStyle {
		t.Fatalf("expected status bar warning style, got %q", pager.statusStyle)
	}
	if state.LastYankTime.IsZero() {
		t.Fatalf("copy should record LastYankTime")
	}
	if copied == "" {
		t.Fatalf("expected clipboard content to be set")
	}
	if msg == "" || !strings.Contains(msg, "copied all") {
		t.Fatalf("expected copy message, got %q", msg)
	}
}

func TestCopyAllBlocksHardLimit(t *testing.T) {
	t.Parallel()
	preview := &statepkg.PreviewData{
		Name:          "huge.txt",
		Size:          clipboardHardLimitBytes + 1,
		TextLines:     []string{"line"},
		LineCount:     1,
		TextCharCount: 4,
	}
	state := &statepkg.AppState{
		CurrentPath:        "/tmp",
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	called := false
	pager.clipboardFunc = func(content string) error {
		called = true
		return nil
	}

	msg, style, err := pager.copyAllToClipboard()
	if err == nil {
		t.Fatalf("expected copy to fail due to size limit")
	}
	if !strings.Contains(err.Error(), "clipboard limit") {
		t.Fatalf("expected clipboard limit error, got %v", err)
	}
	if called {
		t.Fatalf("clipboard should not be invoked when over limit")
	}
	pager.recordCopyResult(err, msg, style)
	if pager.statusStyle != statusErrorStyle {
		t.Fatalf("expected error style, got %q", pager.statusStyle)
	}
	if !state.LastYankTime.IsZero() {
		t.Fatalf("expected LastYankTime to remain zero on failure")
	}
}

func TestCopyVisibleStripsANSIFromFormatted(t *testing.T) {
	t.Parallel()
	preview := &statepkg.PreviewData{
		Name:              "doc.md",
		TextLines:         []string{"# Heading"},
		FormattedSegments: [][]statepkg.StyledTextSegment{{{Text: "# Heading", Style: statepkg.TextStyleHeading}}},
	}
	state := &statepkg.AppState{
		CurrentPath:        "/tmp",
		PreviewData:        preview,
		ClipboardAvailable: true,
	}
	pager, err := NewPreviewPager(state, nil, nil, []string{"clip"})
	if err != nil {
		t.Fatalf("NewPreviewPager: %v", err)
	}
	pager.width = 40
	pager.height = 4

	var copied string
	pager.clipboardFunc = func(content string) error {
		copied = content
		return nil
	}

	if done := pager.handleKey(keyEvent{kind: keyCopyVisible}); done {
		t.Fatalf("copy action should not exit pager")
	}

	if strings.Contains(copied, "\x1b") {
		t.Fatalf("copied content should strip ANSI, got %q", copied)
	}
	if strings.Contains(copied, "?[") {
		t.Fatalf("copied content should not include sanitized escape markers, got %q", copied)
	}
	if strings.TrimSpace(copied) != "# Heading" {
		t.Fatalf("unexpected copied content: %q", copied)
	}
}
