package pager

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	renderpkg "github.com/kk-code-lab/rdir/internal/ui/render"
	"github.com/rivo/uniseg"
	"golang.org/x/term"
)

const (
	binaryPreviewLineWidth  = 16
	binaryPagerChunkSize    = 64 * 1024
	binaryPagerMaxChunks    = 8
	headerBarStyle          = "\x1b[48;5;238m\x1b[97m"
	statusBarStyle          = "\x1b[48;5;236m\x1b[97m"
	statusSuccessStyle      = "\x1b[48;5;22m\x1b[97m"
	statusErrorStyle        = "\x1b[48;5;52m\x1b[97m"
	statusWarnStyle         = "\x1b[48;5;178m\x1b[30m"
	binaryJumpSmallBytes    = 4 * 1024
	binaryJumpLargeBytes    = 64 * 1024
	clipboardWarnBytes      = int64(16 * 1024 * 1024)
	clipboardHardLimitBytes = int64(128 * 1024 * 1024)
	shiftScrollLines        = 10
	searchHighlightOn       = "\x1b[38;5;16;48;5;255m"
	searchHighlightOff      = "\x1b[0m"
	searchHighlightFocusOn  = "\x1b[38;5;16;48;5;178m"
	searchHighlightFocusOff = "\x1b[0m"
	searchDebounceDelay     = 140 * time.Millisecond
)

var (
	searchMaxHits              = 10000
	searchMaxLines             = 20000
	searchMaxBinaryBytes int64 = 16 * 1024 * 1024
)

type pagerContentKind int

const (
	pagerContentUnknown pagerContentKind = iota
	pagerContentText
	pagerContentMarkdown
	pagerContentJSON
	pagerContentBinary
)

func (p *PreviewPager) contentKind() pagerContentKind {
	if p == nil || p.state == nil || p.state.PreviewData == nil {
		return pagerContentUnknown
	}
	preview := p.state.PreviewData
	switch {
	case len(preview.BinaryInfo.Lines) > 0:
		return pagerContentBinary
	case preview.FormattedKind == "markdown":
		return pagerContentMarkdown
	case len(preview.FormattedTextLines) > 0:
		name := strings.ToLower(filepath.Ext(preview.Name))
		if name == ".json" {
			return pagerContentJSON
		}
		return pagerContentText
	case len(preview.TextLines) > 0 || preview.LineCount > 0:
		return pagerContentText
	default:
		return pagerContentUnknown
	}
}

func contentKindLabel(kind pagerContentKind) string {
	switch kind {
	case pagerContentBinary:
		return "binary"
	case pagerContentMarkdown:
		return "markdown"
	case pagerContentJSON:
		return "json"
	case pagerContentText:
		return "text"
	default:
		return "file"
	}
}

type textSpan struct {
	start int
	end   int
}

type searchHit struct {
	line      int
	span      textSpan
	len       int
	startByte int
	nibbleEnd bool
	nibblePos int
}

var termGetSize = term.GetSize

type PreviewPager struct {
	state               *statepkg.AppState
	editorCmd           []string
	reducer             *statepkg.StateReducer
	input               *os.File
	outputFile          *os.File
	output              io.Writer
	reader              *bufio.Reader
	writer              *bufio.Writer
	restoreTerm         *term.State
	stopKeyReader       func()
	width               int
	height              int
	wrapEnabled         bool
	lines               []string
	lineWidths          []int
	rawLines            []string
	rawLineWidths       []int
	rawSanitized        []string
	rawSanitizedWid     []int
	formattedLines      []string
	formattedWidths     []int
	formattedRules      []bool
	formattedStyles     []string
	rowSpans            []int
	rowPrefix           []int
	rowMetricsWidth     int
	charCount           int
	binaryMode          bool
	binarySource        *binaryPagerSource
	rawTextSource       *textPagerSource
	preloadLines        int
	showInfo            bool
	showHelp            bool
	showFormatted       bool
	statusMessage       string
	statusStyle         string
	statusExpiry        time.Time
	statusTimer         *time.Timer
	lastErr             error
	restartKeys         bool
	clipboardCmd        []string
	clipboardFunc       func(string) error
	searchMode          bool
	searchInput         []rune
	searchQuery         string
	searchHits          []searchHit
	searchCursor        int
	searchHighlights    map[int][]textSpan
	searchLimited       bool
	searchErr           error
	searchTimer         *time.Timer
	searchFocused       bool
	searchBinaryMode    bool
	searchQueryBinary   bool
	searchQueryFullScan bool
	searchFullScan      bool

	wrapCacheWidth     int
	wrapCacheFormatted bool
	wrapCacheNextLine  int
	wrapCacheLines     []wrapLineCache

}

var pagerCommand = exec.Command
var clipboardCommand = exec.Command

func NewPreviewPager(state *statepkg.AppState, editorCmd []string, reducer *statepkg.StateReducer, clipboardCmd []string) (*PreviewPager, error) {
	if state == nil || state.PreviewData == nil {
		return nil, errors.New("preview data unavailable")
	}
	pager := &PreviewPager{
		state:        state,
		wrapEnabled:  state.PreviewWrap,
		editorCmd:    append([]string(nil), editorCmd...),
		reducer:      reducer,
		clipboardCmd: append([]string(nil), clipboardCmd...),
		searchCursor: -1,
	}
	pager.prepareContent()
	return pager, nil
}

func (p *PreviewPager) prepareContent() {
	lines, charCount, binarySource, textSource := p.buildContentLines()
	if binarySource != nil {
		p.binaryMode = true
		p.wrapEnabled = false
		p.binarySource = binarySource
		p.rawTextSource = nil
		p.lines = nil
		p.lineWidths = nil
		p.charCount = charCount
		return
	}
	p.binaryMode = false
	p.binarySource = nil
	p.rawTextSource = textSource

	if textSource != nil {
		p.lines = nil
		p.lineWidths = nil
		p.rawLines = nil
		p.rawLineWidths = nil
		p.rawSanitized = nil
		p.rawSanitizedWid = nil
		if p.state != nil && p.state.PreviewScrollOffset > 0 {
			// Preload up to the remembered scroll position so reopening the pager
			// lands where the user left off, even when the file was previously only
			// partially streamed.
			_ = textSource.EnsureLine(p.state.PreviewScrollOffset)
		}
		p.charCount = textSource.CharCount()
	} else {
		if len(lines) == 0 {
			lines = []string{""}
		}
		widths := make([]int, len(lines))
		sanitized := make([]string, len(lines))
		sanitizedWidths := make([]int, len(lines))
		for i, line := range lines {
			widths[i] = displayWidth(line)
			safe := textutil.SanitizeTerminalText(line)
			sanitized[i] = safe
			sanitizedWidths[i] = displayWidth(safe)
		}
		p.lines = lines
		p.lineWidths = widths
		p.rawLines = lines
		p.rawLineWidths = widths
		p.rawSanitized = sanitized
		p.rawSanitizedWid = sanitizedWidths
		p.charCount = charCount
	}

	if preview := p.state.PreviewData; preview != nil {
		if len(preview.FormattedSegments) > 0 {
			formatted := make([]string, len(preview.FormattedSegments))
			widths := make([]int, len(preview.FormattedSegments))
			rules := make([]bool, len(preview.FormattedSegments))
			ruleStyles := make([]string, len(preview.FormattedSegments))
			for i, line := range preview.FormattedSegments {
				formatted[i], rules[i], ruleStyles[i] = ansiFromSegments(line)
				if i < len(preview.FormattedSegmentLineMeta) && preview.FormattedSegmentLineMeta[i].DisplayWidth > 0 {
					widths[i] = preview.FormattedSegmentLineMeta[i].DisplayWidth
				} else {
					widths[i] = segmentDisplayWidth(line)
				}
			}
			p.formattedLines = formatted
			p.formattedWidths = widths
			p.formattedRules = rules
			p.formattedStyles = ruleStyles
		} else if len(preview.FormattedTextLines) > 0 {
			p.formattedLines = append([]string(nil), preview.FormattedTextLines...)
			p.formattedWidths = make([]int, len(p.formattedLines))
			for i, line := range p.formattedLines {
				p.formattedWidths[i] = displayWidth(line)
			}
			p.formattedRules = nil
			p.formattedStyles = nil
		} else {
			p.formattedLines = nil
			p.formattedWidths = nil
			p.formattedRules = nil
			p.formattedStyles = nil
		}
	}

	p.applyFormatPreference(true)
}

func (p *PreviewPager) applyFormatPreference(initial bool) {
	preferRaw := p.state != nil && p.state.PreviewPreferRaw
	if len(p.formattedLines) == 0 {
		p.showFormatted = false
	} else {
		p.showFormatted = !preferRaw
	}
	p.updateDisplayLines()
}

func (p *PreviewPager) updateDisplayLines() {
	if p.showFormatted {
		p.lines = p.formattedLines
		p.lineWidths = p.formattedWidths
	} else {
		if len(p.rawSanitized) > 0 {
			p.lines = p.rawSanitized
			p.lineWidths = p.rawSanitizedWid
		} else {
			p.lines = p.rawLines
			p.lineWidths = p.rawLineWidths
		}
	}
}

func (p *PreviewPager) toggleFormatView() {
	if len(p.formattedLines) == 0 {
		return
	}
	p.showFormatted = !p.showFormatted
	if p.state != nil {
		p.state.PreviewPreferRaw = !p.showFormatted
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
	}
	p.updateDisplayLines()
	p.rowSpans = nil
	p.rowPrefix = nil
	p.resetWrapCache()
	if p.searchQuery != "" {
		p.executeSearch(p.searchQuery)
	}
}

func (p *PreviewPager) Run() error {
	if err := p.initTerminal(); err != nil {
		return err
	}
	defer p.cleanupTerminal()
	defer p.persistLoadedLines()
	defer p.syncBinaryPositionOnExit()

	done := make(chan struct{})
	defer close(done)

	resizeEvents := p.startResizeWatcher(done)
	var keyEvents <-chan keyEvent
	var keyErrs <-chan error
	keyEvents, keyErrs, p.stopKeyReader = p.startKeyReader(done)
	if keyEvents == nil {
		keyEvents, keyErrs, p.stopKeyReader = p.startLocalKeyReader(done)
	}

	p.updateSize()
	p.applyWrapSetting()
	p.syncBinaryPositionOnEnter()
	needsRender := true
	for {
		if needsRender {
			if err := p.render(); err != nil {
				return err
			}
			needsRender = false
		}

		if p.restartKeys {
			if p.stopKeyReader != nil {
				p.stopKeyReader()
			}
			keyEvents, keyErrs, p.stopKeyReader = p.startKeyReader(done)
			if keyEvents == nil {
				keyEvents, keyErrs, p.stopKeyReader = p.startLocalKeyReader(done)
			}
			p.restartKeys = false
		}

		if resizeEvents == nil && keyEvents == nil {
			if p.statusTimer != nil {
				select {
				case <-p.statusTimer.C:
					p.clearStatusMessage()
				default:
				}
			}
			if ch := p.searchTimerC(); ch != nil {
				select {
				case <-ch:
					p.runPendingSearch()
				default:
				}
			}
			event, err := p.readKeyEvent()
			if err != nil {
				return err
			}
			if done := p.handleKey(event); done {
				return p.lastErr
			}
			p.syncBinaryByteOffsetFromScroll()
			needsRender = true
			continue
		}

		select {
		case <-resizeEvents:
			needsRender = true
		case event := <-keyEvents:
			if done := p.handleKey(event); done {
				return p.lastErr
			}
			p.syncBinaryByteOffsetFromScroll()
			needsRender = true
		case err := <-keyErrs:
			if err != nil {
				return err
			}
			return nil
		case <-p.statusTimerC():
			p.clearStatusMessage()
			needsRender = true
		case <-p.searchTimerC():
			p.runPendingSearch()
			needsRender = true
		}
	}
}

// persistLoadedLines copies the portion of the streaming text source that has
// been read so far back into PreviewData so the inline (non-fullscreen) preview
// can display the area the user just viewed in the pager.
// It is intentionally lightweight: we only copy lines already fetched; display
// metadata is omitted so the renderer will measure widths lazily.
func (p *PreviewPager) persistLoadedLines() {
	if p == nil || p.state == nil || p.state.PreviewData == nil || p.rawTextSource == nil {
		return
	}
	count := p.rawTextSource.LineCount()
	if count == 0 {
		return
	}
	lines := make([]string, count)
	metas := make([]statepkg.TextLineMetadata, count)
	for i := 0; i < count; i++ {
		lines[i] = textutil.SanitizeTerminalText(p.rawTextSource.Line(i))
		if i < len(p.rawTextSource.lines) {
			rec := p.rawTextSource.lines[i]
			metas[i] = statepkg.TextLineMetadata{
				Offset:       rec.offset,
				Length:       rec.length,
				RuneCount:    rec.runeCount,
				DisplayWidth: rec.displayWidth,
			}
		}
	}
	preview := p.state.PreviewData
	preview.TextLines = lines
	preview.TextLineMeta = metas
	preview.FormattedTextLines = nil
	preview.FormattedSegments = nil
	preview.FormattedSegmentLineMeta = nil
	preview.LineCount = count
	preview.TextCharCount = p.rawTextSource.CharCount()
	if preview.TextCharCount < 0 {
		preview.TextCharCount = 0
	}
	preview.TextBytesRead = p.rawTextSource.nextOffset
	if len(p.rawTextSource.partialLine) > 0 {
		preview.TextRemainder = append([]byte(nil), p.rawTextSource.partialLine...)
	} else {
		preview.TextRemainder = nil
	}
	preview.TextEncoding = p.rawTextSource.encoding
	preview.TextTruncated = !p.rawTextSource.FullyLoaded()
}

func (p *PreviewPager) startResizeWatcher(done <-chan struct{}) <-chan struct{} {
	signals := resizeSignals()
	if len(signals) == 0 {
		return nil
	}
	resizeCh := make(chan struct{}, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)
	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-done:
				return
			case _, ok := <-sigCh:
				if !ok {
					return
				}
				select {
				case resizeCh <- struct{}{}:
				default:
				}
			}
		}
	}()
	return resizeCh
}

// startLocalKeyReader provides a minimal key reader when platform-specific
// implementations are unavailable (e.g., stub builds). It runs readKeyEvent in a
// goroutine and feeds a channel that can participate in the main select loop.
func (p *PreviewPager) startLocalKeyReader(done <-chan struct{}) (<-chan keyEvent, <-chan error, func()) {
	if p == nil {
		return nil, nil, nil
	}

	input := p.input
	closeInput := false

	if tty := fallbackTTYPath(); tty != "" {
		if file, err := os.OpenFile(tty, os.O_RDONLY, 0); err == nil {
			input = file
			closeInput = true
		}
	}
	if input == nil {
		return nil, nil, nil
	}

	reader := bufio.NewReader(input)
	events := make(chan keyEvent, 1)
	errCh := make(chan error, 1)

	var once sync.Once
	stop := func() {
		once.Do(func() {
			if closeInput {
				_ = input.Close()
			}
		})
	}

	go func() {
		defer close(events)
		defer close(errCh)
		defer stop()

		origReader := p.reader
		p.reader = reader
		defer func() {
			p.reader = origReader
		}()

		for {
			select {
			case <-done:
				return
			default:
			}
			ev, err := p.readKeyEvent()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			select {
			case <-done:
				return
			case events <- ev:
			}
		}
	}()

	go func() {
		<-done
		stop()
	}()

	return events, errCh, stop
}

func fallbackTTYPath() string {
	if runtime.GOOS == "windows" {
		return "CONIN$"
	}
	return "/dev/tty"
}

func (p *PreviewPager) initTerminal() error {
	if p.state == nil || p.state.PreviewData == nil {
		return errors.New("preview data unavailable")
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		if runtime.GOOS == "windows" {
			p.input = os.Stdin
			p.output = os.Stdout
			p.outputFile = os.Stdout
		} else {
			return err
		}
	} else {
		p.input = tty
		p.output = tty
		p.outputFile = tty
	}

	if p.input == nil {
		return errors.New("no tty available")
	}

	p.reader = bufio.NewReader(p.input)
	p.writer = bufio.NewWriter(p.output)

	rawState, err := term.MakeRaw(int(p.input.Fd()))
	if err != nil {
		return err
	}
	p.restoreTerm = rawState
	return nil
}

func (p *PreviewPager) cleanupTerminal() {
	if p.binarySource != nil {
		p.binarySource.Close()
	}
	if p.rawTextSource != nil {
		p.rawTextSource.Close()
	}
	if p.statusTimer != nil {
		p.statusTimer.Stop()
	}
	if p.searchTimer != nil {
		p.searchTimer.Stop()
	}
	if p.input != nil && p.restoreTerm != nil {
		_ = term.Restore(int(p.input.Fd()), p.restoreTerm)
	}
	if p.writer != nil {
		_ = p.writer.Flush()
	}
	if p.writer != nil {
		p.writeString("\x1b[?25h")
		p.writeString("\x1b[?7h")
		_ = p.writer.Flush()
	} else {
		p.writeString("\x1b[?25h")
		p.writeString("\x1b[?7h")
	}
	if p.input != nil && p.input.Name() == "/dev/tty" {
		_ = p.input.Close()
	}
}

func (p *PreviewPager) writeString(s string) {
	switch {
	case p.writer != nil:
		_, _ = p.writer.WriteString(s)
	case p.output != nil:
		_, _ = fmt.Fprint(p.output, s)
	}
}

func (p *PreviewPager) printf(format string, args ...interface{}) {
	switch {
	case p.writer != nil:
		_, _ = fmt.Fprintf(p.writer, format, args...)
	case p.output != nil:
		_, _ = fmt.Fprintf(p.output, format, args...)
	}
}

func (p *PreviewPager) updateSize() {
	oldWidth := p.width
	_ = p.tryUpdateSizeFromFile(p.input)
	if p.outputFile != nil && p.outputFile != p.input {
		_ = p.tryUpdateSizeFromFile(p.outputFile)
	}
	if p.width != oldWidth {
		p.resetWrapCache()
		p.rowMetricsWidth = 0
	}

	if p.binarySource != nil && p.width > 0 {
		oldBytesPerLine := p.binarySource.bytesPerLine
		p.binarySource.UpdateBytesPerLine(p.width)
		newBytesPerLine := p.binarySource.bytesPerLine
		if p.state != nil && oldBytesPerLine > 0 && newBytesPerLine > 0 && oldBytesPerLine != newBytesPerLine {
			byteOffset := p.state.PreviewBinaryByteOffset
			if byteOffset <= 0 {
				byteOffset = int64(p.state.PreviewScrollOffset) * int64(oldBytesPerLine)
			}
			p.state.PreviewBinaryByteOffset = byteOffset
			p.state.PreviewScrollOffset = int(byteOffset / int64(newBytesPerLine))
			p.state.PreviewWrapOffset = 0
			if p.searchQuery != "" {
				p.executeSearch(p.searchQuery)
			}
		}
	}
}

func (p *PreviewPager) binaryBytesPerLine() int {
	if p == nil || p.binarySource == nil || p.binarySource.bytesPerLine <= 0 {
		return binaryPreviewLineWidth
	}
	return p.binarySource.bytesPerLine
}

func (p *PreviewPager) syncBinaryByteOffsetFromScroll() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	bytesPerLine := p.binaryBytesPerLine()
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
	}
	p.state.PreviewBinaryByteOffset = int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
}

func (p *PreviewPager) syncBinaryScrollFromByteOffset(byteOffset int64) {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	if byteOffset < 0 {
		byteOffset = 0
	}
	bytesPerLine := p.binaryBytesPerLine()
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	p.state.PreviewScrollOffset = int(byteOffset / int64(bytesPerLine))
	p.state.PreviewWrapOffset = 0
}

func (p *PreviewPager) syncBinaryPositionOnEnter() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	byteOffset := p.state.PreviewBinaryByteOffset
	if byteOffset <= 0 && p.state.PreviewScrollOffset > 0 {
		// The inline (non-pager) binary preview uses the fixed hexdump width.
		byteOffset = int64(p.state.PreviewScrollOffset) * int64(binaryPreviewLineWidth)
	}
	p.state.PreviewBinaryByteOffset = byteOffset
	p.syncBinaryScrollFromByteOffset(byteOffset)
}

func (p *PreviewPager) syncBinaryPositionOnExit() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	p.state.PreviewWrapOffset = 0

	// Refresh the inline binary preview window around the last byte offset so the
	// non-fullscreen panel doesn't end up showing only the last cached line.
	if p.state.PreviewData != nil && p.binarySource != nil && p.state.CurrentPath != "" && p.state.PreviewData.Name != "" {
		filePath := filepath.Join(p.state.CurrentPath, p.state.PreviewData.Name)
		p.refreshInlineBinaryPreview(filePath, p.binarySource.totalBytes, p.state.PreviewBinaryByteOffset)
		p.state.PreviewScrollOffset = 0
		return
	}

	// Inline preview only has a lightweight set of hexdump lines; if we leave the
	// scroll position pointing beyond that slice, the panel will render empty.
	if p.state.PreviewData != nil && len(p.state.PreviewData.BinaryInfo.Lines) > 0 {
		max := len(p.state.PreviewData.BinaryInfo.Lines) - 1
		if max < 0 {
			max = 0
		}
		if p.state.PreviewScrollOffset > max {
			p.state.PreviewScrollOffset = max
		}
		if p.state.PreviewScrollOffset < 0 {
			p.state.PreviewScrollOffset = 0
		}
		return
	}
	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
	}
}

func (p *PreviewPager) refreshInlineBinaryPreview(path string, totalBytes int64, byteOffset int64) {
	if p == nil || p.state == nil || p.state.PreviewData == nil || path == "" || totalBytes <= 0 {
		return
	}

	const maxBytes = 1024
	bytesPerLine := binaryPreviewLineWidth

	start := byteOffset
	if start < 0 {
		start = 0
	}
	start = (start / int64(bytesPerLine)) * int64(bytesPerLine)
	if start >= totalBytes {
		start = totalBytes - 1
		if start < 0 {
			start = 0
		}
		start = (start / int64(bytesPerLine)) * int64(bytesPerLine)
	}

	readLen := int64(maxBytes)
	if remaining := totalBytes - start; remaining < readLen {
		readLen = remaining
	}
	if readLen <= 0 {
		return
	}

	file := p.binarySource.file
	closeFile := false
	if file == nil {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		file = f
		closeFile = true
	}
	if closeFile {
		defer func() { _ = file.Close() }()
	}

	buf := make([]byte, readLen)
	n, err := file.ReadAt(buf, start)
	if n <= 0 {
		return
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	buf = buf[:n]

	lines := make([]string, 0, (len(buf)+bytesPerLine-1)/bytesPerLine+2)
	if start > 0 {
		lines = append(lines, fmt.Sprintf("… (showing from %s)", formatHexOffset(start)))
	}
	for off := 0; off < len(buf); off += bytesPerLine {
		end := off + bytesPerLine
		if end > len(buf) {
			end = len(buf)
		}
		lines = append(lines, formatHexLine(int(start)+off, buf[off:end], bytesPerLine))
	}
	if tail := totalBytes - (start + int64(len(buf))); tail > 0 {
		lines = append(lines, fmt.Sprintf("… (%d bytes not shown)", tail))
	}

	p.state.PreviewData.BinaryInfo.Lines = lines
	p.state.PreviewData.BinaryInfo.ByteCount = len(buf)
	p.state.PreviewData.BinaryInfo.TotalBytes = totalBytes
}

func (p *PreviewPager) tryUpdateSizeFromFile(file *os.File) bool {
	if file == nil {
		return false
	}
	width, height, err := termGetSize(int(file.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return false
	}
	p.width = width
	p.height = height
	return true
}

func (p *PreviewPager) render() error {
	p.updateSize()
	if p.width <= 0 {
		p.width = 1
	}
	if p.height <= 0 {
		p.height = 1
	}

	p.reflowMarkdownFormatted()

	p.ensureRowMetrics()

	if p.showHelp {
		return p.renderHelpOverlay()
	}

	header := p.headerLines()
	headerRows := len(header)
	if headerRows >= p.height {
		headerRows = p.height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	searchDisplay, cursorColBase := p.searchDisplaySegment()
	showSearchRow := false
	if searchDisplay != "" && p.searchMode {
		available := p.height - headerRows - 1 // must leave space for status
		if available >= 2 {
			showSearchRow = true
		}
	}

	contentRows := p.height - headerRows - 1 // leave space for status
	if showSearchRow {
		contentRows--
	}
	if contentRows < 1 {
		contentRows = 1
		showSearchRow = false
	}
	contentRowLimit := p.height - 1
	if showSearchRow {
		contentRowLimit--
	}
	if contentRowLimit < 1 {
		contentRowLimit = 1
	}

	if !p.showFormatted && p.rawTextSource != nil {
		target := p.state.PreviewScrollOffset + contentRows + 2
		if target < 0 {
			target = 0
		}
		p.preloadLines = target
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	p.writeString("\x1b[?25l")
	p.writeString("\x1b[2J")
	p.writeString("\x1b[H")

	row := 1
	for _, line := range header {
		if row > p.height-1 {
			break
		}
		p.drawStyledRow(row, line, false, headerBarStyle)
		row++
	}

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
		p.state.PreviewScrollOffset = start
	}
	skipRows := 0
	if p.wrapEnabled {
		skipRows = p.state.PreviewWrapOffset
	}

	for i := start; i < totalLines && row <= contentRowLimit; i++ {
		text := p.lineAt(i)
		if p.wrapEnabled && p.width > 0 {
			currentSkip := skipRows
			maxRows := contentRowLimit - row + 1
			segments := p.wrapSegmentsRangeForLine(i, text, currentSkip, maxRows)
			for segIdx, seg := range segments {
				dropCols := (currentSkip + segIdx) * p.width
				if spans, focus := p.visibleHighlights(i, dropCols, p.width); len(spans) > 0 {
					seg = applySearchHighlights(seg, spans, focus)
				}
				p.drawRow(row, seg, false)
				row++
				if row > contentRowLimit {
					break
				}
			}
			skipRows = 0
			continue
		}

		displayText := text
		if p.width > 0 {
			displayText = truncateToWidth(displayText, p.width)
		}
		if spans, focus := p.visibleHighlights(i, 0, p.width); len(spans) > 0 {
			displayText = applySearchHighlights(displayText, spans, focus)
		}
		p.drawRow(row, displayText, false)
		row++
		skipRows = 0
	}

	for row <= contentRowLimit {
		p.drawRow(row, "", false)
		row++
	}

	searchCursorRow := 0
	searchCursorCol := 0
	searchRow := contentRowLimit + 1
	if showSearchRow {
		searchDisplayPadded := " " + searchDisplay + " "
		if p.width > 0 && displayWidth(searchDisplayPadded) > p.width {
			searchDisplayPadded = truncateToWidth(searchDisplayPadded, p.width)
		}
		p.drawStyledRow(searchRow, searchDisplayPadded, true, statusBarStyle)
		searchCursorRow = searchRow
		searchCursorCol = cursorColBase + 1 // account for leading pad
		if searchCursorCol <= 0 {
			searchCursorCol = displayWidth(searchDisplayPadded)
		}
		if p.width > 0 && searchCursorCol > p.width {
			searchCursorCol = p.width
		}
	}

	status := p.statusLine(totalLines, contentRows, p.totalCharCount(), func() string {
		if showSearchRow {
			return ""
		}
		return searchDisplay
	}())
	p.drawStatus(status)
	if !showSearchRow && searchDisplay != "" && p.searchMode {
		searchCursorRow = p.height
		// search is first segment in statusLine; drawStatus wraps with leading space.
		searchCursorCol = cursorColBase + 1 // leading pad
		if searchCursorCol <= 0 {
			searchCursorCol = displayWidth(" " + searchDisplay)
		}
		if p.width > 0 && searchCursorCol > p.width {
			searchCursorCol = p.width
		}
	}
	if searchCursorRow > 0 && p.searchMode {
		p.writeString("\x1b[?25h")
		if searchCursorCol < 1 {
			searchCursorCol = 1
		}
		p.printf("\x1b[%d;%dH", searchCursorRow, searchCursorCol)
	} else {
		p.writeString("\x1b[?25l")
	}

	if p.writer != nil {
		return p.writer.Flush()
	}
	return nil
}

func (p *PreviewPager) renderHelpOverlay() error {
	p.writeString("\x1b[?25l")
	p.writeString("\x1b[2J")
	p.writeString("\x1b[H")

	row := 1
	p.drawStyledRow(row, " Pager shortcuts ", true, headerBarStyle)
	row++

	helpLines := p.helpOverlayLines()
	maxRow := p.height - 1
	if maxRow < row {
		maxRow = row
	}
	for _, line := range helpLines {
		if row >= maxRow {
			break
		}
		p.drawRow(row, line, false)
		row++
	}
	for row < maxRow {
		p.drawRow(row, "", false)
		row++
	}

	p.drawStatus("j/k/PgUp/PgDn scroll  ·  q/Esc/← close")

	if p.writer != nil {
		return p.writer.Flush()
	}
	return nil
}

func (p *PreviewPager) drawRow(row int, text string, bold bool) {
	if row < 1 {
		row = 1
	}
	if row > p.height {
		return
	}

	p.printf("\x1b[%d;1H", row)
	p.writeString("\x1b[0m\x1b[2K")
	if bold {
		p.writeString("\x1b[1m")
	}

	renderText := text
	if !p.wrapEnabled && p.width > 0 {
		renderText = truncateToWidth(text, p.width)
	}
	p.writeString(renderText)

	if bold {
		p.writeString("\x1b[22m")
	}
}

func (p *PreviewPager) reflowMarkdownFormatted() {
	if p.state == nil || p.state.PreviewData == nil {
		return
	}
	preview := p.state.PreviewData
	if preview.FormattedKind != "markdown" || len(preview.TextLines) == 0 || p.width <= 0 {
		return
	}
	maxLines := 3
	if p.wrapEnabled {
		maxLines = 0
	}
	segments, meta := statepkg.FormatMarkdownPreview(preview.TextLines, p.width, maxLines, p.wrapEnabled)
	if len(segments) == 0 || len(meta) != len(segments) {
		return
	}

	formatted := make([]string, len(segments))
	widths := make([]int, len(segments))
	rules := make([]bool, len(segments))
	styles := make([]string, len(segments))
	for i, line := range segments {
		txt, isRule, style := ansiFromSegments(line)
		formatted[i] = txt
		rules[i] = isRule
		styles[i] = style
		if i < len(meta) && meta[i].DisplayWidth > 0 {
			widths[i] = meta[i].DisplayWidth
		} else {
			widths[i] = segmentDisplayWidth(line)
		}
	}

	p.formattedLines = formatted
	p.formattedWidths = widths
	p.formattedRules = rules
	p.formattedStyles = styles

	if p.showFormatted {
		p.lines = p.formattedLines
		p.lineWidths = p.formattedWidths
		p.rowSpans = nil
		p.rowPrefix = nil
	}
}

func (p *PreviewPager) drawStyledRow(row int, text string, bold bool, style string) {
	if row < 1 {
		row = 1
	}
	if row > p.height {
		return
	}

	p.printf("\x1b[%d;1H", row)
	p.writeString("\x1b[0m\x1b[2K")

	if style != "" {
		p.writeString(style)
	}
	if bold {
		p.writeString("\x1b[1m")
	}

	renderText := text
	available := p.width
	if available < 0 {
		available = 0
	}

	needsPadding := style == headerBarStyle
	if needsPadding && available >= 2 {
		bodyWidth := available - 2
		clipped, truncated := clipTextToWidth(renderText, bodyWidth)
		renderText = clipped
		p.writeString(" " + renderText)
		renderWidth := displayWidth(renderText)
		padding := bodyWidth - renderWidth
		if padding > 0 {
			p.writeString(strings.Repeat(" ", padding))
		}
		if truncated {
			p.writeString("…")
		} else {
			p.writeString(" ")
		}
	} else {
		if available > 0 {
			renderText = truncateToWidth(renderText, available)
		}
		p.writeString(renderText)
	}

	if bold {
		p.writeString("\x1b[22m")
	}
	if style != "" || bold {
		p.writeString("\x1b[0m")
	}
}

func (p *PreviewPager) drawStatus(text string) {
	if p.height < 1 {
		return
	}
	display := " " + text + " "
	if p.width > 0 && displayWidth(display) > p.width {
		display = truncateToWidth(display, p.width)
	}
	style := statusBarStyle
	if strings.TrimSpace(p.statusMessage) != "" && p.statusStyle != "" {
		style = p.statusStyle
	}
	p.drawStyledRow(p.height, display, false, style)
}

func (p *PreviewPager) clampScroll(totalLines, visible int) {
	if visible < 1 {
		visible = 1
	}
	if totalLines < 0 {
		totalLines = 0
	}
	if totalLines == 0 {
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
		return
	}
	if !p.wrapEnabled {
		if p.state.PreviewScrollOffset < 0 {
			p.state.PreviewScrollOffset = 0
		}
		maxOffset := totalLines - visible
		if maxOffset < 0 {
			maxOffset = 0
		}
		if p.state.PreviewScrollOffset > maxOffset {
			p.state.PreviewScrollOffset = maxOffset
		}
		p.state.PreviewWrapOffset = 0
		return
	}

	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
	} else if p.state.PreviewScrollOffset >= totalLines {
		p.state.PreviewScrollOffset = totalLines - 1
		if p.state.PreviewScrollOffset < 0 {
			p.state.PreviewScrollOffset = 0
		}
		p.state.PreviewWrapOffset = 0
	}

	rows := p.rowSpanForIndex(p.state.PreviewScrollOffset)
	if rows < 1 {
		rows = 1
	}
	if p.state.PreviewWrapOffset >= rows {
		p.state.PreviewWrapOffset = rows - 1
	}
	if p.state.PreviewWrapOffset < 0 {
		p.state.PreviewWrapOffset = 0
	}

	totalRows := p.totalRowCount()
	maxStart := totalRows - visible
	if maxStart < 0 {
		maxStart = 0
	}
	current := p.currentRowNumber()
	if current > maxStart {
		lineIdx, rowOffset := p.positionFromRow(maxStart)
		p.state.PreviewScrollOffset = lineIdx
		p.state.PreviewWrapOffset = rowOffset
	}
}

func (p *PreviewPager) jumpBinary(deltaBytes int64, stepBytes int64) {
	if p == nil || !p.binaryMode || p.state == nil {
		return
	}
	bytesPerLine := binaryPreviewLineWidth
	if p.binarySource != nil && p.binarySource.bytesPerLine > 0 {
		bytesPerLine = p.binarySource.bytesPerLine
	}
	totalBytes := int64(0)
	if p.binarySource != nil && p.binarySource.totalBytes > 0 {
		totalBytes = p.binarySource.totalBytes
	} else if p.state.PreviewData != nil {
		totalBytes = p.state.PreviewData.BinaryInfo.TotalBytes
		if totalBytes == 0 {
			totalBytes = p.state.PreviewData.Size
		}
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	currentOffset := int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
	target := currentOffset + deltaBytes

	maxOffset := totalBytes - int64(bytesPerLine)
	if maxOffset < 0 {
		maxOffset = 0
	}
	clamped := false
	if target < 0 {
		target = 0
		clamped = true
	}
	if totalBytes > 0 && target > maxOffset {
		target = maxOffset
		clamped = true
	}

	newLine := int(target / int64(bytesPerLine))
	if newLine < 0 {
		newLine = 0
	}
	p.state.PreviewScrollOffset = newLine
	p.state.PreviewWrapOffset = 0

	applied := target - currentOffset
	if applied != 0 || clamped {
		percent := p.binaryProgressPercent(target, totalBytes)
		var direction string
		if applied > 0 {
			direction = "+"
		} else if applied < 0 {
			direction = "-"
		} else {
			direction = ""
		}
		step := stepBytes
		if step < 0 {
			step = -step
		}
		sizeLabel := fmt.Sprintf("%.0f KB", float64(step)/1024.0)
		msg := fmt.Sprintf("jumped %s%s → %s (%d%%)", direction, sizeLabel, formatHexOffset(target), percent)
		if clamped && applied == 0 {
			msg = "at file boundary"
		}
		p.setStatusMessage(msg, "")
	}
}

func (p *PreviewPager) binaryProgressPercent(offset, total int64) int {
	if total <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		offset = total - 1
	}
	return int((offset * 100) / total)
}

func (p *PreviewPager) handleKey(ev keyEvent) bool {
	p.lastErr = nil

	if p.showHelp {
		switch ev.kind {
		case keyToggleHelp, keyQuit, keyEscape, keyLeft:
			p.showHelp = false
		case keyCtrlC:
			return true
		}
		return false
	}

	contentRows := p.height - (len(p.headerLines()) + 1) - 1
	if contentRows < 1 {
		contentRows = 1
	}
	if !p.showFormatted && p.rawTextSource != nil {
		target := p.state.PreviewScrollOffset + contentRows + 2
		if target < 0 {
			target = 0
		}
		p.preloadLines = target
	}

	totalLines := p.lineCount()
	if p.wrapEnabled {
		p.ensureRowMetrics()
	}

	if p.searchMode {
		p.handleSearchModeEvent(ev)
		p.clampScroll(totalLines, contentRows)
		return false
	}

	switch ev.kind {
	case keyQuit, keyEscape, keyCtrlC, keyLeft:
		return true
	case keyToggleHelp:
		p.showHelp = !p.showHelp
	case keyOpenEditor:
		if err := p.openInEditor(); err != nil {
			p.lastErr = err
			return true
		}
		totalLines = p.lineCount()
		if p.wrapEnabled {
			p.ensureRowMetrics()
		}
	case keyUp:
		if p.wrapEnabled {
			p.scrollRows(totalLines, -1)
		} else {
			p.state.PreviewScrollOffset--
		}
	case keyShiftUp:
		if p.wrapEnabled {
			p.scrollRows(totalLines, -shiftScrollLines)
		} else {
			p.state.PreviewScrollOffset -= shiftScrollLines
		}
	case keyDown:
		if p.wrapEnabled {
			p.scrollRows(totalLines, 1)
		} else {
			p.state.PreviewScrollOffset++
		}
	case keyShiftDown:
		if p.wrapEnabled {
			p.scrollRows(totalLines, shiftScrollLines)
		} else {
			p.state.PreviewScrollOffset += shiftScrollLines
		}
	case keyPageUp:
		if p.wrapEnabled {
			p.scrollRows(totalLines, -contentRows)
		} else {
			p.state.PreviewScrollOffset -= contentRows
		}
	case keyPageDown:
		if p.wrapEnabled {
			p.scrollRows(totalLines, contentRows)
		} else {
			p.state.PreviewScrollOffset += contentRows
		}
	case keyJumpBackSmall:
		if p.binaryMode {
			p.jumpBinary(-binaryJumpSmallBytes, binaryJumpSmallBytes)
		} else if p.wrapEnabled {
			p.state.PreviewScrollOffset--
			p.state.PreviewWrapOffset = 0
		}
	case keyJumpForwardSmall:
		if p.binaryMode {
			p.jumpBinary(binaryJumpSmallBytes, binaryJumpSmallBytes)
		} else if p.wrapEnabled {
			p.state.PreviewScrollOffset++
			p.state.PreviewWrapOffset = 0
		}
	case keyJumpBackLarge:
		if p.binaryMode {
			p.jumpBinary(-binaryJumpLargeBytes, binaryJumpLargeBytes)
		}
	case keyJumpForwardLarge:
		if p.binaryMode {
			p.jumpBinary(binaryJumpLargeBytes, binaryJumpLargeBytes)
		}
	case keyHome:
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
	case keyEnd:
		p.scrollToEnd(totalLines)
		totalLines = p.lineCount()
	case keyToggleWrap, keyRight:
		if p.binaryMode {
			break
		}
		p.wrapEnabled = !p.wrapEnabled
		p.state.PreviewWrap = p.wrapEnabled
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
		p.rowMetricsWidth = 0
		p.resetWrapCache()
		p.applyWrapSetting()
	case keySpace:
		if p.wrapEnabled {
			p.scrollRows(totalLines, contentRows)
		} else {
			p.state.PreviewScrollOffset += contentRows
		}
	case keyEnter:
		if p.searchQuery != "" && !p.searchMode && len(p.searchHits) > 0 {
			p.focusSearchHit(p.searchCursor)
			break
		}
		if p.wrapEnabled {
			p.scrollRows(totalLines, contentRows)
		} else {
			p.state.PreviewScrollOffset += contentRows
		}
	case keyToggleInfo:
		p.showInfo = !p.showInfo
	case keyToggleFormat:
		p.toggleFormatView()
	case keyCopyVisible:
		p.recordCopyResult(p.copyVisibleToClipboard(), "copied view", "")
	case keyCopyAll:
		msg, style, err := p.copyAllToClipboard()
		if msg == "" {
			msg = "copied all"
		}
		p.recordCopyResult(err, msg, style)
	case keyStartSearch:
		p.enterTextSearchMode()
	case keyStartBinarySearch:
		p.enterBinarySearchMode()
	case keySearchNext:
		if p.searchQuery != "" || p.searchMode {
			p.moveSearchCursor(1)
		}
	case keySearchPrev:
		if p.searchQuery != "" || p.searchMode {
			p.moveSearchCursor(-1)
		}
	case keyBackspace:
		if p.searchMode {
			p.backspaceSearch()
		}
	case keyRune:
		if p.searchMode {
			p.appendSearchRune(ev.ch)
		}
	case keyToggleBinarySearchMode:
		if p.searchMode {
			p.toggleSearchBinaryMode()
		}
	case keyToggleBinarySearchLimit:
		if p.searchMode {
			p.toggleSearchLimit()
		}
	}

	p.clampScroll(totalLines, contentRows)
	return false
}

func (p *PreviewPager) handleSearchModeEvent(ev keyEvent) {
	switch ev.kind {
	case keyEscape:
		p.cancelSearch()
		return
	case keyToggleBinarySearchMode:
		p.toggleSearchBinaryMode()
		return
	case keyToggleBinarySearchLimit:
		p.toggleSearchLimit()
		return
	case keyLeft:
		if len(p.searchInput) > 0 {
			p.searchInput = nil
			p.onSearchInputChanged()
			return
		}
		p.cancelSearch()
		return
	case keyEnter:
		p.finalizeSearchInput()
		p.exitSearchMode()
		if len(p.searchHits) > 0 {
			p.focusSearchHit(p.searchCursor)
		}
		return
	case keyBackspace:
		p.backspaceSearch()
		return
	}

	switch ev.kind {
	case keySearchNext, keyDown:
		if ev.ch == 0 {
			p.finalizeSearchInput()
			p.moveSearchCursor(1)
			return
		}
	case keySearchPrev, keyUp:
		if ev.ch == 0 {
			p.finalizeSearchInput()
			p.moveSearchCursor(-1)
			return
		}
	}

	if ev.ch != 0 && ev.kind != keyStartSearch {
		p.appendSearchRune(ev.ch)
		return
	}
}

func (p *PreviewPager) canOpenEditor() bool {
	if p == nil || p.state == nil || p.state.PreviewData == nil {
		return false
	}
	if p.state.PreviewData.IsDir {
		return false
	}
	if len(p.editorCmd) == 0 || !p.state.EditorAvailable {
		return false
	}
	return true
}

func (p *PreviewPager) openInEditor() error {
	if p == nil || len(p.editorCmd) == 0 {
		return nil
	}
	if p.state == nil || p.state.PreviewData == nil || p.state.PreviewData.IsDir {
		return nil
	}
	if p.state != nil && !p.state.EditorAvailable {
		return nil
	}
	if p.input == nil {
		return errors.New("no tty available")
	}

	savedScroll := p.state.PreviewScrollOffset
	savedWrap := p.state.PreviewWrapOffset

	filePath := filepath.Join(p.state.CurrentPath, p.state.PreviewData.Name)

	if p.stopKeyReader != nil {
		p.stopKeyReader()
		p.stopKeyReader = nil
	}

	if p.rawTextSource != nil {
		p.rawTextSource.Close()
		p.rawTextSource = nil
	}
	if p.binarySource != nil {
		p.binarySource.Close()
		p.binarySource = nil
	}

	if p.restoreTerm != nil {
		_ = term.Restore(int(p.input.Fd()), p.restoreTerm)
	}
	p.writeString("\x1b[?25h")
	p.writeString("\x1b[?7h")
	if p.writer != nil {
		_ = p.writer.Flush()
	}

	args := append([]string(nil), p.editorCmd...)
	args = append(args, filePath)
	cmd := pagerCommand(args[0], args[1:]...)
	cmd.Stdin = p.input
	cmd.Stdout = p.output
	cmd.Stderr = p.output

	err := cmd.Run()

	if err2 := p.enterPagerMode(); err == nil && err2 != nil {
		err = err2
	}

	if err == nil {
		reducer := p.reducer
		if reducer == nil {
			reducer = statepkg.NewStateReducer()
		}
		if genErr := reducer.GeneratePreview(p.state); genErr != nil {
			err = genErr
		} else {
			p.restoreAfterEditor(savedScroll, savedWrap)
		}
	}

	p.restartKeys = true

	return err
}

// enterPagerMode re-enters raw terminal mode after returning from an external
// editor and reapplies pager-specific terminal settings.
func (p *PreviewPager) enterPagerMode() error {
	if p.input == nil {
		return errors.New("no tty available")
	}

	rawState, rawErr := term.MakeRaw(int(p.input.Fd()))
	if rawErr == nil {
		p.restoreTerm = rawState
	}

	if p.reader != nil {
		p.reader.Reset(p.input)
	} else {
		p.reader = bufio.NewReader(p.input)
	}
	if p.writer != nil {
		p.writer.Reset(p.output)
	} else {
		p.writer = bufio.NewWriter(p.output)
	}

	p.applyWrapSetting()
	p.writeString("\x1b[?25l")
	if p.writer != nil {
		_ = p.writer.Flush()
	}

	return rawErr
}

// restoreAfterEditor rebuilds display buffers and restores scroll position for
// streaming previews after returning from an external editor.
func (p *PreviewPager) restoreAfterEditor(savedScroll, savedWrap int) {
	p.prepareContent()
	p.state.PreviewScrollOffset = savedScroll
	p.state.PreviewWrapOffset = savedWrap
	if !p.showFormatted && p.rawTextSource != nil {
		_ = p.rawTextSource.EnsureLine(savedScroll + 1)
	}
	if p.wrapEnabled {
		p.ensureRowMetrics()
	}
	visible := p.height - (len(p.headerLines()) + 1) - 1
	if visible < 1 {
		visible = 1
	}
	totalLines := p.lineCount()
	p.clampScroll(totalLines, visible)
}

func (p *PreviewPager) statusLine(totalLines, visible, charCount int, search string) string {
	lineApprox := p.isLineCountApprox()
	charApprox := p.isCharCountApprox()
	kind := p.contentKind()

	segments := []string{p.positionSegment(totalLines, visible, lineApprox)}
	if count := p.countSegment(kind, charCount, charApprox); count != "" {
		segments = append(segments, count)
	}
	if offset := p.binaryOffsetSegment(); offset != "" {
		segments = append(segments, offset)
	}
	segments = append(segments, p.statusBadges(kind)...)
	if search != "" {
		segments = append([]string{search}, segments...)
	}
	segments = filterEmptyStrings(segments)

	base := strings.Join(segments, "  ")
	help := strings.Join(p.helpSegments(), "  ")

	if msg := strings.TrimSpace(p.statusMessage); msg != "" {
		msg = textutil.SanitizeTerminalText(msg)
		if base != "" {
			msg += "  " + base
		}
		if help != "" {
			msg += "  " + help
		}
		return msg
	}

	if help != "" {
		if base != "" {
			base += "  "
		}
		base += help
	}
	return base
}

func (p *PreviewPager) positionSegment(totalLines, visible int, approx bool) string {
	if visible < 1 {
		visible = 1
	}
	if p.wrapEnabled {
		totalRows := p.totalRowCount()
		startRow := 0
		if totalRows > 0 {
			startRow = p.currentRowNumber() + 1
			if startRow > totalRows {
				startRow = totalRows
			}
		}
		endRow := startRow + visible - 1
		if endRow > totalRows {
			endRow = totalRows
		}
		percent := p.progressPercent(startRow, totalRows)
		linesText := fmt.Sprintf("%d lines", totalLines)
		if approx {
			linesText = fmt.Sprintf("~%s", linesText)
		}
		return fmt.Sprintf("%d-%d/%d rows (%s, %d%%)", startRow, endRow, totalRows, linesText, percent)
	}

	lineLabel := "lines"
	if p.binaryMode {
		lineLabel = "rows"
	}
	start := 0
	if totalLines > 0 {
		start = p.state.PreviewScrollOffset + 1
		if start > totalLines {
			start = totalLines
		}
	}
	end := start + visible - 1
	if end > totalLines {
		end = totalLines
	}
	percent := p.progressPercent(start, totalLines)
	linesText := fmt.Sprintf("%d-%d/%d %s", start, end, totalLines, lineLabel)
	if approx && !p.binaryMode {
		linesText = fmt.Sprintf("%d-%d/~%d %s", start, end, totalLines, lineLabel)
	}
	return fmt.Sprintf("%s (%d%%)", linesText, percent)
}

func (p *PreviewPager) countSegment(kind pagerContentKind, charCount int, approx bool) string {
	if charCount <= 0 {
		return ""
	}
	prefix := ""
	if approx {
		prefix = "~"
	}
	if kind == pagerContentBinary {
		return fmt.Sprintf("%s%d bytes", prefix, charCount)
	}
	return fmt.Sprintf("%s%d chars", prefix, charCount)
}

func (p *PreviewPager) binaryOffsetSegment() string {
	if p == nil || !p.binaryMode || p.state == nil {
		return ""
	}
	bytesPerLine := binaryPreviewLineWidth
	totalBytes := int64(0)
	if p.binarySource != nil {
		if p.binarySource.bytesPerLine > 0 {
			bytesPerLine = p.binarySource.bytesPerLine
		}
		totalBytes = p.binarySource.totalBytes
	}
	if totalBytes == 0 && p.state.PreviewData != nil {
		totalBytes = p.state.PreviewData.BinaryInfo.TotalBytes
		if totalBytes == 0 {
			totalBytes = p.state.PreviewData.Size
		}
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	offset := int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
	if totalBytes <= 0 {
		return fmt.Sprintf("offset: %s", formatHexOffset(offset))
	}
	percent := p.binaryProgressPercent(offset, totalBytes)
	return fmt.Sprintf("offset: %s/%s  pos: %d%%", formatHexOffset(offset), formatHexOffset(totalBytes), percent)
}

func (p *PreviewPager) statusBadges(kind pagerContentKind) []string {
	preview := (*statepkg.PreviewData)(nil)
	if p.state != nil {
		preview = p.state.PreviewData
	}
	badges := []string{}
	if label := contentKindLabel(kind); label != "" {
		badges = append(badges, "type:"+label)
	}
	if !p.binaryMode {
		wrap := "off"
		if p.wrapEnabled {
			wrap = "on"
		}
		badges = append(badges, "wrap:"+wrap)
	}
	formattedAvailable := len(p.formattedLines) > 0
	formattedReason := preview != nil && preview.FormattedUnavailableReason != ""
	if formattedAvailable || formattedReason {
		mode := "raw"
		if formattedAvailable && p.showFormatted {
			mode = "pretty"
		} else if formattedReason && !formattedAvailable {
			mode = "raw*"
		}
		badges = append(badges, "fmt:"+mode)
	}
	if preview != nil && preview.HiddenFormattingDetected && !p.binaryMode {
		badges = append(badges, "hidden:yes")
	}
	infoState := "off"
	if p.showInfo {
		infoState = "on"
	}
	badges = append(badges, "info:"+infoState)
	if p.binaryMode && p.effectiveBinaryFullScan() {
		badges = append(badges, "scan:*")
	}
	return badges
}

func (p *PreviewPager) effectiveBinaryFullScan() bool {
	if p == nil || !p.binaryMode {
		return false
	}
	if p.searchMode {
		return p.searchFullScan
	}
	return p.searchQueryFullScan
}

func (p *PreviewPager) searchStatusSegment() string {
	segment, _ := p.searchDisplaySegment()
	return segment
}

func (p *PreviewPager) searchDisplaySegment() (string, int) {
	if p == nil {
		return "", 0
	}
	displayRaw := p.searchQuery
	binary := false
	if p.searchMode {
		binary = p.searchBinaryMode
	} else if p.binaryMode && p.searchQueryBinary {
		binary = true
	}
	prefix := "/"
	if binary {
		prefix = ":"
	}
	displayText := displayRaw
	if binary && strings.HasPrefix(displayText, ":") {
		displayText = strings.TrimPrefix(displayText, ":")
	}
	if p.searchMode {
		displayRaw = string(p.searchInput)
		displayText = displayRaw
		if binary && strings.HasPrefix(displayText, ":") {
			displayText = strings.TrimPrefix(displayText, ":")
		}
	}
	if displayText == "" {
		if p.searchMode {
			return prefix, displayWidth(prefix) + 1
		}
		return "", 0
	}

	visibleQuery := visualizeSpaces(displayText)
	safeQuery := textutil.SanitizeTerminalText(visibleQuery)
	segment := prefix + safeQuery
	cursorCol := displayWidth(segment) + 1

	activeQuery := p.searchQuery
	matchInput := displayRaw
	useResults := activeQuery != "" && strings.EqualFold(activeQuery, matchInput)

	if !p.searchMode {
		if activeQuery == "" {
			return "", 0
		}
		if !useResults {
			return segment, 0
		}
		if p.searchErr != nil {
			return segment + " !", 0
		}
		counts := p.searchCountsSegment()
		if binary && p.effectiveBinaryFullScan() {
			counts += "*"
		}
		return segment + " " + counts, 0
	}

	if useResults {
		counts := p.searchCountsSegment()
		if binary && p.effectiveBinaryFullScan() {
			counts += "*"
		}
		segment += " " + counts
	}
	return segment, cursorCol
}

func (p *PreviewPager) searchCountsSegment() string {
	total := len(p.searchHits)
	if total == 0 {
		if p.searchLimited {
			return "0/0+"
		}
		return "0/0"
	}
	cursor := p.searchCursor
	if cursor < 0 || cursor >= total {
		cursor = 0
	}
	seg := fmt.Sprintf("%d/%d", cursor+1, total)
	if p.searchLimited {
		seg += "+"
	}
	return seg
}

type helpEntry struct {
	keys string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func (p *PreviewPager) helpOverlayLines() []string {
	width := p.width
	if width <= 0 {
		width = 80
	}
	available := width
	if available > 2 {
		available -= 2
	}
	lines := []string{}
	useSeparator := width >= 60
	separator := helpSeparator(available)
	for _, section := range p.helpSections() {
		if len(section.entries) == 0 {
			continue
		}
		if len(lines) > 0 {
			if useSeparator && separator != "" {
				lines = append(lines, separator)
			} else {
				lines = append(lines, "")
			}
		}
		lines = append(lines, section.title+":")
		lines = append(lines, formatHelpEntries(section.entries, available)...)
	}
	return lines
}

func helpSeparator(width int) string {
	if width < 4 {
		return ""
	}
	maxWidth := width
	if maxWidth > 60 {
		maxWidth = 60
	}
	return strings.Repeat("-", maxWidth)
}

func formatHelpEntries(entries []helpEntry, width int) []string {
	maxKeys := 0
	for _, entry := range entries {
		if w := displayWidth(entry.keys); w > maxKeys {
			maxKeys = w
		}
	}
	padding := maxKeys + 2
	if padding < 2 {
		padding = 2
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line := fmt.Sprintf("  %-*s %s", padding, entry.keys, entry.desc)
		if width > 0 && displayWidth(line) > width {
			line = truncateToWidth(line, width)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *PreviewPager) helpSections() []helpSection {
	nav := []helpEntry{
		{keys: "↑/↓ or j/k", desc: "Scroll one line"},
		{keys: "Shift+↑/↓", desc: "Jump ±10 lines"},
		{keys: "PgUp / b", desc: "Page up"},
		{keys: "PgDn / space", desc: "Page down"},
		{keys: "Home/End or g/G", desc: "Jump to start/end"},
	}
	if p.binaryMode {
		nav = append(nav,
			helpEntry{keys: "[ / ]", desc: "Jump ±4 KB"},
			helpEntry{keys: "{ / }", desc: "Jump ±64 KB"},
		)
	} else if p.wrapEnabled {
		nav = append(nav, helpEntry{keys: "[ / ]", desc: "Skip wrapped line"})
	}

	view := []helpEntry{
		{keys: "?", desc: "Toggle this help"},
		{keys: "i", desc: "Toggle info line"},
	}
	if !p.binaryMode {
		view = append(view, helpEntry{keys: "w or →", desc: "Toggle wrap"})
	}
	if len(p.formattedLines) > 0 {
		view = append(view, helpEntry{keys: "f", desc: "Toggle formatted view"})
	}

	actions := []helpEntry{}
	if p.clipboardAvailable() {
		actions = append(actions,
			helpEntry{keys: "c", desc: "Copy visible lines"},
			helpEntry{keys: "C", desc: "Copy entire file (raw)"},
		)
	}
	if p.canOpenEditor() {
		actions = append(actions, helpEntry{keys: "e", desc: "Open in editor"})
	}
	actions = append(actions, helpEntry{keys: "Ctrl+C", desc: "Quit immediately"})

	exit := []helpEntry{
		{keys: "← / q / x / Esc", desc: "Exit pager"},
	}

	search := []helpEntry{
		{keys: "/", desc: "Enter search"},
		{keys: "n / N", desc: "Jump to next/prev hit"},
	}
	if p.binaryMode {
		search = append(search, helpEntry{keys: ":", desc: "Enter binary search"})
		search = append(search, helpEntry{keys: "Ctrl+B", desc: "Toggle text/hex mode while searching"})
		search = append(search, helpEntry{keys: "Ctrl+L", desc: "Toggle full scan for binary search"})
	}

	sections := []helpSection{
		{title: "Navigation", entries: nav},
		{title: "View", entries: view},
	}
	if len(search) > 0 {
		sections = append(sections, helpSection{title: "Search", entries: search})
	}
	sections = append(sections,
		helpSection{title: "Actions", entries: actions},
		helpSection{title: "Exit", entries: exit},
	)
	return sections
}

func (p *PreviewPager) helpSegments() []string {
	return []string{"? help"}
}

func (p *PreviewPager) clipboardAvailable() bool {
	if p == nil {
		return false
	}
	if p.state == nil || !p.state.ClipboardAvailable {
		return false
	}
	if p.clipboardFunc != nil {
		return true
	}
	return len(p.clipboardCmd) > 0
}

func (p *PreviewPager) recordCopyResult(err error, successMsg string, style string) {
	if err != nil {
		p.setStatusMessage(err.Error(), statusErrorStyle)
		if p.state != nil {
			p.state.LastError = err
		}
		return
	}
	if style == "" {
		style = statusSuccessStyle
	}
	p.setStatusMessage(successMsg, style)
	if p.state != nil {
		p.state.LastYankTime = time.Now()
	}
}

func (p *PreviewPager) setStatusMessage(msg string, style string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		p.statusMessage = ""
		p.statusStyle = ""
		p.stopStatusTimer()
		return
	}
	p.statusMessage = textutil.SanitizeTerminalText(msg)
	p.statusStyle = style
	p.startStatusTimer(1500 * time.Millisecond)
}

func (p *PreviewPager) copyVisibleToClipboard() error {
	lines := p.visibleContentLinesForCopy()
	return p.copyLinesToClipboard(lines)
}

func (p *PreviewPager) copyAllToClipboard() (string, string, error) {
	if !p.clipboardAvailable() {
		return "", "", errors.New("clipboard unavailable")
	}

	size := p.clipboardByteSize()
	if size > 0 && size >= clipboardHardLimitBytes {
		return "", "", fmt.Errorf("copy canceled: %s exceeds clipboard limit (%s)", formatSize(size), formatSize(clipboardHardLimitBytes))
	}

	if !p.showFormatted && p.rawTextSource != nil {
		if err := p.rawTextSource.EnsureAll(); err != nil {
			return "", "", err
		}
	}

	warn := size > 0 && size >= clipboardWarnBytes
	if warn {
		p.setStatusMessage(fmt.Sprintf("copying %s; this may be slow", formatSize(size)), statusWarnStyle)
	}

	preferRawCopy := p.showFormatted && !p.binaryMode && (p.rawTextSource != nil || len(p.rawLines) > 0)

	if p.clipboardFunc != nil {
		var builder strings.Builder
		if preferRawCopy {
			if err := p.writeAllLinesRaw(&builder); err != nil {
				return "", "", err
			}
		} else {
			if err := p.writeAllLines(&builder); err != nil {
				return "", "", err
			}
		}
		if err := p.clipboardFunc(builder.String()); err != nil {
			return "", "", err
		}
	} else {
		if preferRawCopy {
			if err := p.streamAllLinesToClipboardRaw(); err != nil {
				return "", "", err
			}
		} else {
			if err := p.streamAllLinesToClipboard(); err != nil {
				return "", "", err
			}
		}
	}

	msg := "copied all"
	if preferRawCopy {
		msg = "copied all (raw)"
	}
	if size > 0 {
		msg = fmt.Sprintf("%s (%s)", msg, formatSize(size))
	}
	if warn {
		return msg, statusWarnStyle, nil
	}
	return msg, "", nil
}

func (p *PreviewPager) visibleContentLines() []string {
	if p == nil {
		return nil
	}
	if p.state == nil {
		return nil
	}
	width := p.width
	if width <= 0 {
		width = 1
	}
	height := p.height
	if height <= 0 {
		height = 1
	}
	headerRows := len(p.headerLines())
	if headerRows >= height {
		headerRows = height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	contentRows := height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
	}
	skipRows := 0
	if p.wrapEnabled {
		skipRows = p.state.PreviewWrapOffset
	}

	rowsRemaining := contentRows
	lines := []string{}
	for i := start; i < totalLines && rowsRemaining > 0; i++ {
		text := lineForClipboard(p.lineAt(i))
		if p.wrapEnabled {
			segments := wrapLineSegments(text, width)
			if skipRows > 0 {
				if skipRows >= len(segments) {
					skipRows = 0
					continue
				}
				segments = segments[skipRows:]
				skipRows = 0
			}
			for _, segment := range segments {
				lines = append(lines, segment)
				rowsRemaining--
				if rowsRemaining == 0 {
					break
				}
			}
			continue
		}

		if width > 0 {
			text = truncateToWidth(text, width)
		}
		lines = append(lines, text)
		rowsRemaining--
	}
	return lines
}

func (p *PreviewPager) visibleContentLinesForCopy() []string {
	if p == nil {
		return nil
	}
	if p.state == nil {
		return nil
	}
	if !p.wrapEnabled || p.width <= 0 {
		return p.visibleContentLines()
	}
	height := p.height
	if height <= 0 {
		height = 1
	}
	headerRows := len(p.headerLines())
	if headerRows >= height {
		headerRows = height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	contentRows := height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
	}
	skipRows := p.state.PreviewWrapOffset

	rowsRemaining := contentRows
	lines := []string{}
	for i := start; i < totalLines && rowsRemaining > 0; i++ {
		text := lineForClipboard(p.lineAt(i))
		segments := p.wrapSegmentsRangeForLine(i, text, skipRows, rowsRemaining)
		if len(segments) == 0 {
			skipRows = 0
			continue
		}
		lines = append(lines, strings.Join(segments, ""))
		rowsRemaining -= len(segments)
		skipRows = 0
	}
	return lines
}

func (p *PreviewPager) copyLinesToClipboard(lines []string) error {
	if !p.clipboardAvailable() {
		return errors.New("clipboard unavailable")
	}
	content := strings.Join(lines, "\n")
	if p.clipboardFunc != nil {
		return p.clipboardFunc(content)
	}
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	if cmd.Stdin == nil {
		cmd.Stdin = strings.NewReader(content)
	}
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	return nil
}

func (p *PreviewPager) writeAllLines(w io.Writer) error {
	if p == nil {
		return errors.New("pager unavailable")
	}
	total := p.lineCount()
	if total < 0 {
		total = 0
	}
	bufw := bufio.NewWriter(w)
	for i := 0; i < total; i++ {
		if _, err := bufw.WriteString(lineForClipboard(p.lineAt(i))); err != nil {
			return err
		}
		if i+1 < total {
			if err := bufw.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	if err := bufw.Flush(); err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) writeAllLinesRaw(w io.Writer) error {
	if p == nil {
		return errors.New("pager unavailable")
	}
	bufw := bufio.NewWriter(w)
	if p.rawTextSource != nil {
		if err := p.rawTextSource.EnsureAll(); err != nil {
			return err
		}
		total := p.rawTextSource.LineCount()
		if total < 0 {
			total = 0
		}
		for i := 0; i < total; i++ {
			if _, err := bufw.WriteString(lineForClipboard(p.rawTextSource.Line(i))); err != nil {
				return err
			}
			if i+1 < total {
				if err := bufw.WriteByte('\n'); err != nil {
					return err
				}
			}
		}
		return bufw.Flush()
	}

	total := len(p.rawLines)
	for i := 0; i < total; i++ {
		if _, err := bufw.WriteString(lineForClipboard(p.rawLines[i])); err != nil {
			return err
		}
		if i+1 < total {
			if err := bufw.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	return bufw.Flush()
}

func (p *PreviewPager) streamAllLinesToClipboard() error {
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	reader, writer := io.Pipe()
	cmd.Stdin = reader
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	writeErrCh := make(chan error, 1)
	go func() {
		err := p.writeAllLines(writer)
		_ = writer.CloseWithError(err)
		writeErrCh <- err
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	if err := <-writeErrCh; err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) streamAllLinesToClipboardRaw() error {
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	reader, writer := io.Pipe()
	cmd.Stdin = reader
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	writeErrCh := make(chan error, 1)
	go func() {
		err := p.writeAllLinesRaw(writer)
		_ = writer.CloseWithError(err)
		writeErrCh <- err
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	if err := <-writeErrCh; err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) clipboardByteSize() int64 {
	if p == nil || p.state == nil || p.state.PreviewData == nil {
		return 0
	}
	// Prefer the stat size when available; fall back to binary source totals.
	if p.state.PreviewData.Size > 0 {
		return p.state.PreviewData.Size
	}
	if p.binarySource != nil && p.binarySource.totalBytes > 0 {
		return p.binarySource.totalBytes
	}
	return 0
}

func wrapLineSegments(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}

	out := []string{}
	for len(text) > 0 {
		consumed := 0
		index := 0
		g := uniseg.NewGraphemes(text)
		for g.Next() {
			cluster := g.Str()
			w := textutil.DisplayWidth(cluster)
			if w <= 0 {
				w = 1
			}
			if consumed+w > width {
				if consumed == 0 {
					index += len(cluster)
				}
				break
			}
			consumed += w
			index += len(cluster)
			if consumed >= width {
				break
			}
		}
		if index <= 0 {
			index = len(text)
		}
		out = append(out, text[:index])
		text = text[index:]
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func wrapLineSegmentsRange(text string, width int, skipRows int, maxRows int) []string {
	if maxRows == 0 {
		return nil
	}
	if width <= 0 {
		if skipRows <= 0 && maxRows != 0 {
			return []string{text}
		}
		return nil
	}
	if text == "" {
		if skipRows <= 0 && maxRows != 0 {
			return []string{""}
		}
		return nil
	}

	out := []string{}
	row := 0
	var b strings.Builder
	consumed := 0
	flush := func() {
		if row >= skipRows {
			out = append(out, b.String())
		}
		row++
		b.Reset()
		consumed = 0
	}

	g := uniseg.NewGraphemes(text)
	for g.Next() {
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w <= 0 {
			w = 1
		}
		if consumed+w > width && consumed > 0 {
			flush()
			if maxRows > 0 && len(out) >= maxRows {
				return out
			}
		}
		b.WriteString(cluster)
		consumed += w
		if consumed >= width {
			flush()
			if maxRows > 0 && len(out) >= maxRows {
				return out
			}
		}
	}
	if b.Len() > 0 {
		flush()
	}
	if maxRows > 0 && len(out) > maxRows {
		out = out[:maxRows]
	}
	return out
}

func lineForClipboard(text string) string {
	if text == "" {
		return ""
	}
	stripped := stripANSICodes(text)
	return textutil.SanitizeTerminalText(stripped)
}

func stripANSICodes(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) && text[j] != 'm' {
				j++
			}
			if j < len(text) {
				j++
			}
			i = j
			continue
		}
		ru, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			i++
			continue
		}
		b.WriteRune(ru)
		i += size
	}
	return b.String()
}

func (p *PreviewPager) startStatusTimer(d time.Duration) {
	if p.statusTimer != nil {
		if !p.statusTimer.Stop() {
			select {
			case <-p.statusTimer.C:
			default:
			}
		}
	}
	p.statusExpiry = time.Now().Add(d)
	p.statusTimer = time.NewTimer(d)
}

func (p *PreviewPager) stopStatusTimer() {
	if p.statusTimer != nil {
		p.statusTimer.Stop()
		p.statusTimer = nil
	}
	p.statusExpiry = time.Time{}
}

func (p *PreviewPager) statusTimerC() <-chan time.Time {
	if p.statusTimer == nil {
		return nil
	}
	return p.statusTimer.C
}

func (p *PreviewPager) clearStatusMessage() {
	p.statusMessage = ""
	p.statusStyle = ""
	p.stopStatusTimer()
}

func filterEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		result = append(result, v)
	}
	return result
}

func (p *PreviewPager) lineCount() int {
	if !p.showFormatted && p.rawTextSource != nil {
		target := p.preloadLines
		min := p.state.PreviewScrollOffset + 1
		if target < min {
			target = min
		}
		if target < 0 {
			target = 0
		}
		_ = p.rawTextSource.EnsureLine(target)
		return p.rawTextSource.LineCount()
	}
	if p.binaryMode {
		if p.binarySource == nil {
			return 0
		}
		return p.binarySource.LineCount()
	}
	return len(p.lines)
}

func (p *PreviewPager) totalCharCount() int {
	if p.rawTextSource != nil {
		return p.rawTextSource.CharCount()
	}
	return p.charCount
}

func (p *PreviewPager) isCharCountApprox() bool {
	if p == nil || p.binaryMode {
		return false
	}
	if p.rawTextSource != nil {
		return !p.rawTextSource.FullyLoaded()
	}
	if p.state != nil && p.state.PreviewData != nil {
		return p.state.PreviewData.TextTruncated
	}
	return false
}

func (p *PreviewPager) isLineCountApprox() bool {
	if p == nil || p.binaryMode {
		return false
	}
	if p.rawTextSource != nil {
		return !p.rawTextSource.FullyLoaded()
	}
	if p.state != nil && p.state.PreviewData != nil {
		return p.state.PreviewData.TextTruncated
	}
	return false
}

func (p *PreviewPager) headerCharCount(preview *statepkg.PreviewData) int {
	if count := p.totalCharCount(); count > 0 {
		return count
	}
	if preview != nil {
		return preview.TextCharCount
	}
	return 0
}

func (p *PreviewPager) headerLineCount(preview *statepkg.PreviewData) int {
	if p.rawTextSource != nil {
		return p.rawTextSource.LineCount()
	}
	if preview != nil {
		return preview.LineCount
	}
	return 0
}

func (p *PreviewPager) lineAt(idx int) string {
	if !p.showFormatted && p.rawTextSource != nil {
		return textutil.SanitizeTerminalText(p.rawTextSource.Line(idx))
	}
	if p.binaryMode {
		if p.binarySource == nil {
			return ""
		}
		return p.binarySource.Line(idx)
	}
	if idx < 0 || idx >= len(p.lines) {
		return ""
	}
	if p.showFormatted && idx < len(p.formattedRules) && p.formattedRules[idx] {
		width := p.width
		if width <= 0 {
			width = displayWidth(p.lines[idx])
			if width <= 0 {
				width = 1
			}
		}
		style := ""
		if idx < len(p.formattedStyles) {
			style = p.formattedStyles[idx]
		}
		reset := ""
		if style != "" {
			reset = "\x1b[0m"
		}
		return style + strings.Repeat("─", width) + reset
	}
	return p.lines[idx]
}

func (p *PreviewPager) progressPercent(position, total int) int {
	if total <= 0 {
		return 0
	}
	if position < 1 {
		position = 1
	}
	if position > total {
		position = total
	}
	return (position*100 + total/2) / total
}

func (p *PreviewPager) headerLines() []string {
	if p.state == nil || p.state.PreviewData == nil {
		return []string{"(no preview available)"}
	}
	preview := p.state.PreviewData
	fullPath := filepath.Join(p.state.CurrentPath, preview.Name)

	lines := []string{textutil.SanitizeTerminalText(fullPath)}
	if p.showInfo {
		if info := p.infoLine(preview); info != "" {
			lines = append(lines, info)
		}
	}
	return lines
}

func (p *PreviewPager) infoLine(preview *statepkg.PreviewData) string {
	segments := p.infoSegments(preview)
	if len(segments) == 0 {
		return ""
	}
	return textutil.SanitizeTerminalText(strings.Join(segments, "  •  "))
}

func (p *PreviewPager) infoSegments(preview *statepkg.PreviewData) []string {
	if preview == nil {
		return nil
	}
	meta := fmt.Sprintf("%s  %s  %s", preview.Mode.String(), formatSize(preview.Size), preview.Modified.Format("2006-01-02 15:04:05"))
	segments := []string{meta}
	segments = append(segments, p.detailInfoSegments(preview)...)
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		out = append(out, seg)
	}
	return out
}

func (p *PreviewPager) detailInfoSegments(preview *statepkg.PreviewData) []string {
	kind := p.contentKind()
	segments := []string{}
	if label := contentKindLabel(kind); label != "" {
		segments = append(segments, "type:"+label)
	}
	switch kind {
	case pagerContentBinary:
		if preview.BinaryInfo.TotalBytes > 0 {
			segments = append(segments, fmt.Sprintf("bytes:%d", preview.BinaryInfo.TotalBytes))
		}
		if preview.LineCount > 0 {
			segments = append(segments, fmt.Sprintf("rows:%d", preview.LineCount))
		}
		segments = append(segments, fmt.Sprintf("%d B/row", binaryPreviewLineWidth))
	default:
		if lineCount := p.headerLineCount(preview); lineCount > 0 {
			lineSegment := fmt.Sprintf("lines:%d", lineCount)
			if p.isLineCountApprox() {
				lineSegment = fmt.Sprintf("lines:~%d", lineCount)
			}
			segments = append(segments, lineSegment)
		}
		if count := p.headerCharCount(preview); count > 0 {
			label := fmt.Sprintf("chars:%d", count)
			if p.isCharCountApprox() {
				label = fmt.Sprintf("chars:~%d", count)
			}
			segments = append(segments, label)
		}
		if enc := formatEncodingLabel(preview.TextEncoding); enc != "" {
			segments = append(segments, "encoding:"+enc)
		}
		if p.rawTextSource != nil {
			if !p.rawTextSource.FullyLoaded() {
				segments = append(segments, "streaming from disk")
			}
		} else if preview.TextTruncated {
			segments = append(segments, "preview truncated")
		}
		if preview.HiddenFormattingDetected {
			segments = append(segments, "hidden formatting detected")
		}
		if preview.FormattedUnavailableReason != "" {
			segments = append(segments, preview.FormattedUnavailableReason)
		}
	}
	return segments
}

func formatEncodingLabel(enc fsutil.UnicodeEncoding) string {
	switch enc {
	case fsutil.EncodingUnknown:
		return "utf-8/ascii"
	case fsutil.EncodingUTF8BOM:
		return "utf-8 bom"
	case fsutil.EncodingUTF16LE:
		return "utf-16le"
	case fsutil.EncodingUTF16BE:
		return "utf-16be"
	default:
		return ""
	}
}

func (p *PreviewPager) applyWrapSetting() {
	if p.output == nil {
		return
	}
	p.writeString("\x1b[?7l")
}

type wrapRowCacheEntry struct {
	row   int
	index int
}

const wrapCacheRowCapacity = 4096
const wrapCacheLineCapacity = 64
const wrapCacheWindowMax = 4096

type wrapLineCache struct {
	line        int
	next        int
	rows        []wrapRowCacheEntry
	windowStart int
	windowRows  []string
}

func (p *PreviewPager) resetWrapCache() {
	p.wrapCacheWidth = 0
	p.wrapCacheFormatted = false
	p.wrapCacheNextLine = 0
	p.wrapCacheLines = nil
}

func (c *wrapLineCache) remember(row int, index int) {
	if row < 0 || index < 0 {
		return
	}
	if len(c.rows) < wrapCacheRowCapacity {
		c.rows = append(c.rows, wrapRowCacheEntry{row: row, index: index})
		return
	}
	c.rows[c.next] = wrapRowCacheEntry{row: row, index: index}
	c.next = (c.next + 1) % wrapCacheRowCapacity
}

func (c *wrapLineCache) findStart(row int) (int, int, bool) {
	bestRow := -1
	bestIdx := 0
	for _, entry := range c.rows {
		if entry.row <= row && entry.row > bestRow {
			bestRow = entry.row
			bestIdx = entry.index
		}
	}
	if bestRow < 0 {
		return 0, 0, false
	}
	return bestRow, bestIdx, true
}

func (p *PreviewPager) wrapCacheForLine(idx int) *wrapLineCache {
	for i := range p.wrapCacheLines {
		if p.wrapCacheLines[i].line == idx {
			return &p.wrapCacheLines[i]
		}
	}
	if len(p.wrapCacheLines) < wrapCacheLineCapacity {
		p.wrapCacheLines = append(p.wrapCacheLines, wrapLineCache{line: idx})
		return &p.wrapCacheLines[len(p.wrapCacheLines)-1]
	}
	p.wrapCacheLines[p.wrapCacheNextLine] = wrapLineCache{line: idx}
	cache := &p.wrapCacheLines[p.wrapCacheNextLine]
	p.wrapCacheNextLine = (p.wrapCacheNextLine + 1) % wrapCacheLineCapacity
	return cache
}

func (p *PreviewPager) wrapSegmentsRangeForLine(idx int, text string, skipRows int, maxRows int) []string {
	if maxRows == 0 {
		return nil
	}
	if p.width <= 0 {
		if skipRows <= 0 {
			return []string{text}
		}
		return nil
	}
	if text == "" {
		if skipRows <= 0 {
			return []string{""}
		}
		return nil
	}

	if p.wrapCacheWidth != p.width || p.wrapCacheFormatted != p.showFormatted {
		p.resetWrapCache()
		p.wrapCacheWidth = p.width
		p.wrapCacheFormatted = p.showFormatted
	}

	lineWidth := p.lineWidth(idx)
	if lineWidth > 0 && p.width > 0 {
		if lineWidth <= p.width {
			if skipRows <= 0 {
				return []string{text}
			}
			return nil
		}
		cache := p.wrapCacheForLine(idx)
		if len(cache.rows) == 0 {
			cache.remember(0, 0)
		}
		if cache.windowRows == nil {
			cache.windowStart = 0
			cache.windowRows = wrapLineSegments(text, p.width)
		}
		start := skipRows
		if start < 0 {
			start = 0
		}
		if start >= len(cache.windowRows) {
			return nil
		}
		end := start + maxRows
		if end > len(cache.windowRows) {
			end = len(cache.windowRows)
		}
		return cache.windowRows[start:end]
	}

	cache := p.wrapCacheForLine(idx)
	if len(cache.rows) == 0 {
		cache.remember(0, 0)
	}
	if cache.windowRows != nil {
		if skipRows >= cache.windowStart && skipRows+maxRows <= cache.windowStart+len(cache.windowRows) {
			start := skipRows - cache.windowStart
			return cache.windowRows[start : start+maxRows]
		}
	}

	windowSize := wrapCacheWindowMax
	if windowSize < maxRows {
		windowSize = maxRows
	}
	windowStart := skipRows - windowSize/2
	if windowStart < 0 {
		windowStart = 0
	}

	startRow := 0
	startIndex := 0
	if r, i, ok := cache.findStart(windowStart); ok {
		startRow = r
		startIndex = i
	}

	row := startRow
	index := startIndex
	window := []string{}

	for index <= len(text) {
		if windowSize > 0 && len(window) >= windowSize && row >= windowStart {
			break
		}
		if index >= len(text) {
			break
		}
		segment, nextIndex := nextWrapSegment(text, index, p.width)
		if row >= windowStart {
			window = append(window, segment)
			if windowSize > 0 && len(window) >= windowSize {
				cache.remember(row+1, nextIndex)
				break
			}
		}
		row++
		cache.remember(row, nextIndex)
		index = nextIndex
	}
	cache.windowStart = windowStart
	cache.windowRows = window

	if maxRows <= 0 {
		return nil
	}
	if skipRows < cache.windowStart {
		return nil
	}
	start := skipRows - cache.windowStart
	if start < 0 || start >= len(window) {
		return nil
	}
	end := start + maxRows
	if end > len(window) {
		end = len(window)
	}
	return window[start:end]
}

func nextWrapSegment(text string, start int, width int) (string, int) {
	if width <= 0 || start >= len(text) {
		return "", start
	}
	consumed := 0
	index := start
	var b strings.Builder
	g := uniseg.NewGraphemes(text[start:])
	for g.Next() {
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w <= 0 {
			w = 1
		}
		if consumed+w > width && consumed > 0 {
			break
		}
		b.WriteString(cluster)
		consumed += w
		index += len(cluster)
		if consumed >= width {
			break
		}
	}
	if b.Len() == 0 && start < len(text) {
		ru, size := utf8.DecodeRuneInString(text[start:])
		if size > 0 && ru != utf8.RuneError {
			b.WriteRune(ru)
			index = start + size
		} else {
			b.WriteByte(text[start])
			index = start + 1
		}
	}
	return b.String(), index
}

func (p *PreviewPager) rowSpanForIndex(idx int) int {
	if p.binaryMode {
		return 1
	}
	if !p.showFormatted && p.rawTextSource != nil {
		if p.wrapEnabled && p.width > 0 && idx >= 0 && idx < len(p.rowSpans) && p.rowMetricsWidth == p.width {
			if span := p.rowSpans[idx]; span > 0 {
				return span
			}
		}
		return p.rowSpanFromWidth(p.lineWidth(idx))
	}
	if idx < 0 || idx >= len(p.lines) {
		return 1
	}
	if p.wrapEnabled && p.width > 0 && len(p.rowSpans) == len(p.lines) && p.rowMetricsWidth == p.width {
		if span := p.rowSpans[idx]; span > 0 {
			return span
		}
	}
	return p.rowSpanFromWidth(p.lineWidth(idx))
}

func (p *PreviewPager) rowSpanFromWidth(width int) int {
	if !p.wrapEnabled || p.width <= 0 {
		return 1
	}
	if width <= 0 {
		return 1
	}
	rows := width / p.width
	if width%p.width != 0 {
		rows++
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (p *PreviewPager) lineWidth(idx int) int {
	if !p.showFormatted && p.rawTextSource != nil {
		return p.rawTextSource.LineWidth(idx)
	}
	if p.binaryMode {
		return displayWidth(p.lineAt(idx))
	}
	if p.showFormatted && idx >= 0 && idx < len(p.formattedRules) && p.formattedRules[idx] {
		if p.width > 0 {
			return p.width
		}
	}
	if idx < 0 || idx >= len(p.lineWidths) {
		return 0
	}
	return p.lineWidths[idx]
}

func (p *PreviewPager) ensureRowMetrics() {
	if p.binaryMode || !p.wrapEnabled || p.width <= 0 {
		p.rowSpans = nil
		p.rowPrefix = nil
		p.rowMetricsWidth = 0
		return
	}
	if !p.showFormatted && p.rawTextSource != nil {
		count := p.lineCount()
		if count == 0 {
			p.rowSpans = nil
			p.rowPrefix = nil
			p.rowMetricsWidth = p.width
			return
		}
		if p.rowMetricsWidth != p.width || len(p.rowPrefix) == 0 {
			p.rowSpans = make([]int, 0, count)
			p.rowPrefix = []int{0}
		}
		for len(p.rowSpans) < count {
			width := p.lineWidth(len(p.rowSpans))
			span := p.rowSpanFromWidth(width)
			p.rowSpans = append(p.rowSpans, span)
			last := p.rowPrefix[len(p.rowPrefix)-1]
			p.rowPrefix = append(p.rowPrefix, last+span)
		}
		p.rowMetricsWidth = p.width
		return
	}
	if len(p.lines) == 0 {
		p.rowSpans = nil
		p.rowPrefix = nil
		p.rowMetricsWidth = 0
		return
	}
	if p.rowMetricsWidth == p.width && len(p.rowSpans) == len(p.lines) {
		return
	}
	p.rowMetricsWidth = p.width
	p.rowSpans = make([]int, len(p.lines))
	p.rowPrefix = make([]int, len(p.lines)+1)
	for i := range p.lines {
		span := p.rowSpanFromWidth(p.lineWidth(i))
		p.rowSpans[i] = span
		p.rowPrefix[i+1] = p.rowPrefix[i] + span
	}
}

func (p *PreviewPager) totalRowCount() int {
	if !p.wrapEnabled || p.width <= 0 {
		return p.lineCount()
	}
	p.ensureRowMetrics()
	if len(p.rowPrefix) == 0 {
		return 0
	}
	return p.rowPrefix[len(p.rowPrefix)-1]
}

func (p *PreviewPager) currentRowNumber() int {
	if !p.wrapEnabled || p.width <= 0 {
		pos := p.state.PreviewScrollOffset
		if pos < 0 {
			return 0
		}
		total := p.lineCount()
		if pos > total {
			pos = total
		}
		return pos
	}
	p.ensureRowMetrics()
	if len(p.rowPrefix) == 0 {
		return 0
	}
	lineIdx := p.state.PreviewScrollOffset
	if lineIdx < 0 {
		return 0
	}
	if lineIdx >= len(p.rowSpans) {
		return p.rowPrefix[len(p.rowPrefix)-1]
	}
	base := p.rowPrefix[lineIdx]
	span := p.rowSpans[lineIdx]
	if span <= 0 {
		span = 1
	}
	offset := p.state.PreviewWrapOffset
	if offset < 0 {
		offset = 0
	}
	if offset >= span {
		offset = span - 1
	}
	return base + offset
}

func (p *PreviewPager) positionFromRow(row int) (int, int) {
	if !p.wrapEnabled || p.width <= 0 {
		if row < 0 {
			return 0, 0
		}
		total := p.lineCount()
		if row >= total {
			last := total - 1
			if last < 0 {
				return 0, 0
			}
			return last, 0
		}
		return row, 0
	}
	p.ensureRowMetrics()
	if len(p.rowPrefix) == 0 {
		return 0, 0
	}
	totalRows := p.rowPrefix[len(p.rowPrefix)-1]
	if totalRows <= 0 {
		return 0, 0
	}
	if row < 0 {
		row = 0
	}
	if row >= totalRows {
		row = totalRows - 1
	}
	idx := sort.Search(len(p.rowPrefix)-1, func(i int) bool {
		return p.rowPrefix[i+1] > row
	})
	if idx >= len(p.rowSpans) {
		idx = len(p.rowSpans) - 1
		if idx < 0 {
			return 0, 0
		}
	}
	offset := row - p.rowPrefix[idx]
	span := p.rowSpans[idx]
	if span <= 0 {
		span = 1
	}
	if offset >= span {
		offset = span - 1
	}
	if offset < 0 {
		offset = 0
	}
	return idx, offset
}

func (p *PreviewPager) trimWrappedPrefix(text string, skipRows int) string {
	if !p.wrapEnabled || p.width <= 0 || skipRows <= 0 || text == "" {
		return text
	}
	target := skipRows * p.width
	if target <= 0 {
		return text
	}
	consumed := 0
	index := 0
	for index < len(text) && consumed < target {
		if text[index] == '\x1b' && index+1 < len(text) && text[index+1] == '[' {
			end := index + 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			index = end
			continue
		}
		g := uniseg.NewGraphemes(text[index:])
		if !g.Next() {
			break
		}
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w < 1 {
			w = 1
		}
		consumed += w
		index += len(cluster)
	}
	if index >= len(text) {
		return ""
	}
	return text[index:]
}

func (p *PreviewPager) scrollRows(totalLines int, delta int) {
	if delta == 0 || totalLines <= 0 {
		return
	}
	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
	} else if p.state.PreviewScrollOffset >= totalLines {
		p.state.PreviewScrollOffset = totalLines - 1
		if p.state.PreviewScrollOffset < 0 {
			p.state.PreviewScrollOffset = 0
		}
		p.state.PreviewWrapOffset = 0
	}
	if delta > 0 {
		for ; delta > 0; delta-- {
			rows := p.rowSpanForIndex(p.state.PreviewScrollOffset)
			if rows <= 0 {
				rows = 1
			}
			if p.state.PreviewWrapOffset < rows-1 {
				p.state.PreviewWrapOffset++
				continue
			}
			if p.state.PreviewScrollOffset >= totalLines-1 {
				p.state.PreviewWrapOffset = rows - 1
				break
			}
			p.state.PreviewScrollOffset++
			p.state.PreviewWrapOffset = 0
		}
		return
	}

	for delta < 0 {
		if p.state.PreviewWrapOffset > 0 {
			p.state.PreviewWrapOffset--
			delta++
			continue
		}
		if p.state.PreviewScrollOffset <= 0 {
			p.state.PreviewScrollOffset = 0
			p.state.PreviewWrapOffset = 0
			return
		}
		p.state.PreviewScrollOffset--
		rows := p.rowSpanForIndex(p.state.PreviewScrollOffset)
		if rows <= 0 {
			rows = 1
		}
		p.state.PreviewWrapOffset = rows - 1
		delta++
	}
}

func (p *PreviewPager) scrollToEnd(totalLines int) {
	if !p.showFormatted && p.rawTextSource != nil {
		_ = p.rawTextSource.EnsureAll()
		totalLines = p.rawTextSource.LineCount()
		p.rowMetricsWidth = 0
	}
	if totalLines <= 0 {
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
		return
	}
	if !p.wrapEnabled {
		p.state.PreviewScrollOffset = totalLines
		p.state.PreviewWrapOffset = 0
		return
	}
	last := totalLines - 1
	p.state.PreviewScrollOffset = last
	rows := p.rowSpanForIndex(last)
	if rows <= 0 {
		rows = 1
	}
	p.state.PreviewWrapOffset = rows - 1
}

func (p *PreviewPager) buildContentLines() ([]string, int, *binaryPagerSource, *textPagerSource) {
	if p.state == nil || p.state.PreviewData == nil {
		return nil, 0, nil, nil
	}

	preview := p.state.PreviewData
	switch {
	case preview.IsDir:
		lines := formatDirectoryPreview(preview)
		return lines, lineCharCount(lines), nil, nil
	case len(preview.TextLines) > 0:
		if preview.TextTruncated && len(preview.TextLineMeta) == len(preview.TextLines) {
			filePath := filepath.Join(p.state.CurrentPath, preview.Name)
			if source, err := newTextPagerSource(filePath, preview); err == nil {
				return nil, preview.TextCharCount, nil, source
			}
		}
		return preview.TextLines, preview.TextCharCount, nil, nil
	case len(preview.BinaryInfo.Lines) > 0:
		filePath := filepath.Join(p.state.CurrentPath, preview.Name)
		source, err := newBinaryPagerSource(filePath, preview.BinaryInfo.TotalBytes, p.width)
		if err == nil {
			return nil, int(preview.BinaryInfo.TotalBytes), source, nil
		}
		lines := append([]string(nil), preview.BinaryInfo.Lines...)
		if len(lines) > 0 {
			lines = lines[1:]
		}
		return lines, lineCharCount(lines), nil, nil
	default:
		lines := []string{"(no preview available)"}
		return lines, lineCharCount(lines), nil, nil
	}
}

type keyKind int

const (
	keyUnknown keyKind = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
	keyEscape
	keyQuit
	keyToggleWrap
	keySpace
	keyCtrlC
	keyToggleHelp
	keyToggleInfo
	keyToggleFormat
	keyOpenEditor
	keyShiftUp
	keyShiftDown
	keyCopyVisible
	keyCopyAll
	keyStartSearch
	keyStartBinarySearch
	keySearchNext
	keySearchPrev
	keyToggleBinarySearchMode
	keyToggleBinarySearchLimit
	keyEnter
	keyBackspace
	keyRune
	keyJumpBackSmall
	keyJumpForwardSmall
	keyJumpBackLarge
	keyJumpForwardLarge
)

type keyEvent struct {
	kind keyKind
	ch   rune
	mod  int
}

func (p *PreviewPager) readKeyEvent() (keyEvent, error) {
	if p.reader == nil {
		return keyEvent{}, errors.New("no reader available")
	}

	b, err := p.reader.ReadByte()
	if err != nil {
		return keyEvent{}, err
	}

	switch b {
	case 0x1b:
		return p.parseEscapeSequence()
	case '?':
		return keyEvent{kind: keyToggleHelp, ch: rune(b)}, nil
	case 'k', 'K':
		return keyEvent{kind: keyUp, ch: rune(b)}, nil
	case 'j', 'J':
		return keyEvent{kind: keyDown, ch: rune(b)}, nil
	case 'h', 'H':
		return keyEvent{kind: keyToggleHelp, ch: rune(b)}, nil
	case 'q', 'Q':
		return keyEvent{kind: keyQuit, ch: rune(b)}, nil
	case 'x', 'X':
		return keyEvent{kind: keyQuit, ch: rune(b)}, nil
	case 'w', 'W':
		return keyEvent{kind: keyToggleWrap, ch: rune(b)}, nil
	case 'i', 'I':
		return keyEvent{kind: keyToggleInfo, ch: rune(b)}, nil
	case 'f', 'F':
		return keyEvent{kind: keyToggleFormat, ch: rune(b)}, nil
	case 'e', 'E':
		return keyEvent{kind: keyOpenEditor, ch: rune(b)}, nil
	case 'c':
		return keyEvent{kind: keyCopyVisible, ch: rune(b)}, nil
	case 'C':
		return keyEvent{kind: keyCopyAll, ch: rune(b)}, nil
	case '/':
		return keyEvent{kind: keyStartSearch, ch: rune(b)}, nil
	case ':':
		return keyEvent{kind: keyStartBinarySearch, ch: rune(b)}, nil
	case 'n':
		return keyEvent{kind: keySearchNext, ch: rune(b)}, nil
	case 'N':
		return keyEvent{kind: keySearchPrev, ch: rune(b)}, nil
	case 0x02: // Ctrl+B
		return keyEvent{kind: keyToggleBinarySearchMode}, nil
	case 0x0c: // Ctrl+L
		return keyEvent{kind: keyToggleBinarySearchLimit}, nil
	case ' ':
		return keyEvent{kind: keySpace, ch: rune(b)}, nil
	case 'b', 'B':
		return keyEvent{kind: keyPageUp, ch: rune(b)}, nil
	case 'g':
		return keyEvent{kind: keyHome, ch: rune(b)}, nil
	case 'G':
		return keyEvent{kind: keyEnd, ch: rune(b)}, nil
	case '[':
		return keyEvent{kind: keyJumpBackSmall, ch: rune(b)}, nil
	case ']':
		return keyEvent{kind: keyJumpForwardSmall, ch: rune(b)}, nil
	case '{':
		return keyEvent{kind: keyJumpBackLarge, ch: rune(b)}, nil
	case '}':
		return keyEvent{kind: keyJumpForwardLarge, ch: rune(b)}, nil
	case '\r', '\n':
		return keyEvent{kind: keyEnter}, nil
	case 0x7f, 0x08:
		return keyEvent{kind: keyBackspace}, nil
	case 0x03:
		return keyEvent{kind: keyCtrlC}, nil
	default:
	}

	if b < utf8.RuneSelf {
		if b >= 0x20 {
			return keyEvent{kind: keyRune, ch: rune(b)}, nil
		}
		return keyEvent{kind: keyUnknown}, nil
	}

	buf := []byte{b}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		next, err := p.reader.ReadByte()
		if err != nil {
			break
		}
		buf = append(buf, next)
	}
	r, _ := utf8.DecodeRune(buf)
	if r != utf8.RuneError {
		return keyEvent{kind: keyRune, ch: r}, nil
	}
	return keyEvent{kind: keyUnknown}, nil
}

func (p *PreviewPager) parseEscapeSequence() (keyEvent, error) {
	if p.reader.Buffered() == 0 {
		return keyEvent{kind: keyEscape}, nil
	}
	next, err := p.reader.ReadByte()
	if err != nil {
		return keyEvent{kind: keyEscape}, nil
	}

	switch next {
	case '[':
		return p.parseCSI()
	case 'O':
		final, err := p.reader.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEscape}, nil
		}
		switch final {
		case 'H':
			return keyEvent{kind: keyHome}, nil
		case 'F':
			return keyEvent{kind: keyEnd}, nil
		default:
			return keyEvent{kind: keyUnknown}, nil
		}
	default:
		return keyEvent{kind: keyEscape}, nil
	}
}

func (p *PreviewPager) parseCSI() (keyEvent, error) {
	seq := []byte{}
	for {
		b, err := p.reader.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEscape}, nil
		}
		seq = append(seq, b)
		if isCSIFinalByte(b) {
			break
		}
		if len(seq) >= 16 {
			return keyEvent{kind: keyUnknown}, nil
		}
	}

	if len(seq) == 0 {
		return keyEvent{kind: keyUnknown}, nil
	}

	final := seq[len(seq)-1]
	base, modifier := parseCSIParameters(string(seq[:len(seq)-1]))

	switch final {
	case 'A':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftUp, mod: modifier}, nil
		}
		return keyEvent{kind: keyUp, mod: modifier}, nil
	case 'B':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftDown, mod: modifier}, nil
		}
		return keyEvent{kind: keyDown, mod: modifier}, nil
	case 'C':
		return keyEvent{kind: keyRight, mod: modifier}, nil
	case 'D':
		return keyEvent{kind: keyLeft, mod: modifier}, nil
	case 'H':
		return keyEvent{kind: keyHome, mod: modifier}, nil
	case 'F':
		return keyEvent{kind: keyEnd, mod: modifier}, nil
	case '~':
		switch base {
		case "5":
			return keyEvent{kind: keyPageUp, mod: modifier}, nil
		case "6":
			return keyEvent{kind: keyPageDown, mod: modifier}, nil
		case "1", "7":
			return keyEvent{kind: keyHome, mod: modifier}, nil
		case "4", "8":
			return keyEvent{kind: keyEnd, mod: modifier}, nil
		default:
			return keyEvent{kind: keyUnknown}, nil
		}
	}
	return keyEvent{kind: keyUnknown}, nil
}

func isCSIFinalByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || b == '~'
}

func parseCSIParameters(param string) (string, int) {
	if param == "" {
		return "", 1
	}

	parts := strings.Split(param, ";")
	if len(parts) == 0 {
		return "", 1
	}

	modifier := 1
	baseParts := parts
	if len(parts) > 1 {
		if val, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			modifier = val
			baseParts = parts[:len(parts)-1]
			if len(baseParts) == 0 {
				baseParts = []string{"1"}
			}
		}
	}
	base := strings.Join(baseParts, ";")
	return base, modifier
}

func hasShiftModifier(mod int) bool {
	switch mod {
	case 2, 4, 6, 8:
		return true
	default:
		return false
	}
}

func formatDirectoryPreview(preview *statepkg.PreviewData) []string {
	if preview == nil || len(preview.DirEntries) == 0 {
		return []string{"(directory is empty)"}
	}

	lines := make([]string, 0, len(preview.DirEntries))
	for _, entry := range preview.DirEntries {
		lines = append(lines, dirEntryLine(entry))
	}
	return lines
}

func dirEntryLine(entry statepkg.FileEntry) string {
	icon := " "
	switch {
	case entry.IsSymlink:
		icon = "@"
	case entry.IsDir:
		icon = "/"
	}
	name := textutil.SanitizeTerminalText(entry.Name)
	size := formatSize(entry.Size)
	mod := entry.Modified.Format("2006-01-02 15:04:05")
	return fmt.Sprintf(" %s %-20s %12s  %s  %s", icon, name, size, entry.Mode.String(), mod)
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func formatHexOffset(offset int64) string {
	if offset < 0 {
		offset = 0
	}
	s := fmt.Sprintf("%08X", offset)
	var b strings.Builder
	b.Grow(len(s) + len(s)/4 + 2)
	b.WriteString("0x")
	for i, ch := range s {
		if i > 0 && (len(s)-i)%4 == 0 {
			b.WriteByte('_')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansiDisplayWidth(text) <= width {
		return text
	}
	truncated, _ := ansiTruncate(text, width, true)
	return truncated
}

func clipTextToWidth(text string, width int) (string, bool) {
	if width <= 0 {
		return "", ansiDisplayWidth(text) > 0
	}
	return ansiTruncate(text, width, false)
}

func displayWidth(text string) int {
	return ansiDisplayWidth(text)
}

func ansiFromSegments(segments []statepkg.StyledTextSegment) (string, bool, string) {
	if len(segments) == 0 {
		return "", false, ""
	}
	rule := true
	for _, seg := range segments {
		if seg.Style != statepkg.TextStyleRule {
			rule = false
			break
		}
	}

	var b strings.Builder
	reset := "\x1b[0m"
	var styleCode string
	for _, seg := range segments {
		text := textutil.SanitizeTerminalText(seg.Text)
		if text == "" {
			continue
		}
		code := ansiForStyle(seg.Style)
		if code != "" {
			b.WriteString(code)
			if styleCode == "" {
				styleCode = code
			}
		}
		b.WriteString(text)
		if seg.Style != statepkg.TextStylePlain {
			b.WriteString(reset)
		}
	}
	return b.String(), rule, styleCode
}

func ansiForStyle(kind statepkg.TextStyleKind) string {
	switch kind {
	case statepkg.TextStyleStrong, statepkg.TextStyleHeading:
		return "\x1b[1m"
	case statepkg.TextStyleEmphasis:
		return "\x1b[3m"
	case statepkg.TextStyleStrike:
		return "\x1b[9m"
	case statepkg.TextStyleCode:
		return ansiColorSequence(pagerTheme.CodeFg, pagerTheme.CodeBg)
	case statepkg.TextStyleCodeBlock:
		return ansiColorSequence(pagerTheme.CodeBlockFg, pagerTheme.CodeBlockBg)
	case statepkg.TextStyleLink:
		return "\x1b[4m"
	case statepkg.TextStyleRule:
		return "\x1b[2m"
	default:
		return ""
	}
}

var pagerTheme = renderpkg.GetColorTheme()

func ansiColorSequence(fg, bg tcell.Color) string {
	if fg == tcell.ColorDefault && bg == tcell.ColorDefault {
		return ""
	}
	parts := make([]string, 0, 2)
	if fg != tcell.ColorDefault {
		r, g, b := fg.RGB()
		parts = append(parts, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if bg != tcell.ColorDefault {
		r, g, b := bg.RGB()
		parts = append(parts, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func segmentDisplayWidth(segments []statepkg.StyledTextSegment) int {
	width := 0
	for _, seg := range segments {
		width += displayWidth(seg.Text)
	}
	return width
}

func ansiDisplayWidth(text string) int {
	width := 0
	for len(text) > 0 {
		esc := strings.IndexByte(text, '\x1b')
		if esc == -1 {
			width += textutil.DisplayWidth(text)
			break
		}
		if esc > 0 {
			width += textutil.DisplayWidth(text[:esc])
		}
		text = text[esc:]
		if len(text) >= 2 && text[0] == '\x1b' && text[1] == '[' {
			end := 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			if end > len(text) {
				end = len(text)
			}
			text = text[end:]
			continue
		}
		text = text[1:]
	}
	return width
}

func ansiTruncate(text string, width int, withEllipsis bool) (string, bool) {
	if width <= 0 {
		return "", ansiDisplayWidth(text) > 0
	}
	const ellipsisRune = '…'
	ellipsisWidth := textutil.DisplayWidth(string(ellipsisRune))
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	target := width
	if withEllipsis {
		target -= ellipsisWidth
	}
	if target < 0 {
		target = 0
	}

	var b strings.Builder
	b.Grow(len(text))
	consumed := 0
	truncated := false

	for len(text) > 0 {
		if text[0] == '\x1b' && len(text) > 1 && text[1] == '[' {
			end := 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			if end > len(text) {
				end = len(text)
			}
			b.WriteString(text[:end])
			text = text[end:]
			continue
		}

		g := uniseg.NewGraphemes(text)
		if !g.Next() {
			break
		}
		cluster := g.Str()
		clusterWidth := textutil.DisplayWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}
		if consumed+clusterWidth > target {
			truncated = true
			break
		}
		b.WriteString(cluster)
		consumed += clusterWidth
		text = text[len(cluster):]
	}

	if !truncated && len(text) > 0 {
		truncated = true
	}

	if truncated && withEllipsis && ellipsisWidth <= width {
		b.WriteRune(ellipsisRune)
	}
	return b.String(), truncated
}

func lineCharCount(lines []string) int {
	total := 0
	for _, line := range lines {
		total += utf8.RuneCountInString(line)
	}
	return total
}

func calculateBytesPerLine(pagerWidth int) int {
	// Calculate optimal bytes per line based on available pager width
	// Format: [8-char offset] [hex bytes] |[ASCII bytes]|
	// Minimum width needed for different byte counts:
	// 8 bytes: 10 (offset) + 8*3 (hex) + 3 (spaces) + 3 (separators) + 8 (ASCII) = 48 chars
	// 16 bytes: 10 (offset) + 16*3 (hex) + 4 (spaces) + 3 (separators) + 16 (ASCII) = 81 chars
	// 24 bytes: 10 (offset) + 24*3 (hex) + 5 (spaces) + 3 (separators) + 24 (ASCII) = 112 chars

	if pagerWidth <= 0 {
		return binaryPreviewLineWidth // fallback to default
	}

	// Use the full width for calculation, be more generous with space usage
	// Most terminals can handle tight layouts, and the separators provide visual structure
	if pagerWidth >= 120 {
		return 24 // 24 bytes per line for wide terminals
	} else if pagerWidth >= 90 {
		return 16 // 16 bytes per line for medium terminals
	} else if pagerWidth >= 60 {
		return 8 // 8 bytes per line for narrow terminals
	} else {
		return binaryPreviewLineWidth // fallback to default if very narrow
	}
}

func formatHexLine(offset int, chunk []byte, bytesPerLine int) string {
	var builder strings.Builder
	// Estimate buffer size: 10 (offset) + bytesPerLine*3 (hex) + bytesPerLine/8 (spaces) + 3 (separators) + bytesPerLine (ASCII)
	builder.Grow(10 + bytesPerLine*3 + bytesPerLine/8 + 3 + bytesPerLine)
	fmt.Fprintf(&builder, "%08X  ", offset)

	for i := 0; i < bytesPerLine; i++ {
		if i < len(chunk) {
			fmt.Fprintf(&builder, "%02X ", chunk[i])
		} else {
			builder.WriteString("   ")
		}
		// Add grouping space every 8 bytes (after byte 7, 15, 23, etc.)
		if (i+1)%8 == 0 && i < bytesPerLine-1 {
			builder.WriteString(" ")
		}
	}

	builder.WriteString(" |")
	for i := 0; i < len(chunk); i++ {
		builder.WriteByte(printableASCII(chunk[i]))
	}
	for i := len(chunk); i < bytesPerLine; i++ {
		builder.WriteByte(' ')
	}
	builder.WriteString("|")
	return builder.String()
}

func printableASCII(b byte) byte {
	if b >= 32 && b <= 126 {
		return b
	}
	return '.'
}

type binaryPagerSource struct {
	path         string
	totalBytes   int64
	bytesPerLine int
	chunkSize    int
	maxChunks    int
	file         *os.File
	cache        map[int]*binaryChunk
	cacheOrder   []int
}

type binaryChunk struct {
	index int
	lines []string
}

func alignedBinaryChunkSize(bytesPerLine int) int {
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	size := binaryPagerChunkSize
	if size <= 0 {
		size = bytesPerLine
	}
	if size < bytesPerLine {
		size = bytesPerLine
	}
	aligned := (size / bytesPerLine) * bytesPerLine
	if aligned < bytesPerLine {
		aligned = bytesPerLine
	}
	return aligned
}

func newBinaryPagerSource(path string, totalBytes int64, pagerWidth int) (*binaryPagerSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	bytesPerLine := calculateBytesPerLine(pagerWidth)

	source := &binaryPagerSource{
		path:         path,
		totalBytes:   totalBytes,
		bytesPerLine: bytesPerLine,
		chunkSize:    alignedBinaryChunkSize(bytesPerLine),
		maxChunks:    binaryPagerMaxChunks,
		file:         file,
		cache:        make(map[int]*binaryChunk),
	}
	return source, nil
}

func (s *binaryPagerSource) Close() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
	s.file = nil
}

func (s *binaryPagerSource) UpdateBytesPerLine(pagerWidth int) {
	if s == nil {
		return
	}
	newBytesPerLine := calculateBytesPerLine(pagerWidth)
	if newBytesPerLine == s.bytesPerLine {
		return // no change needed
	}

	// Clear cache since line formatting will change
	s.cache = make(map[int]*binaryChunk)
	s.cacheOrder = nil
	s.bytesPerLine = newBytesPerLine
	s.chunkSize = alignedBinaryChunkSize(newBytesPerLine)
}

func (s *binaryPagerSource) LineCount() int {
	if s == nil {
		return 0
	}
	return s.dataLineCount()
}

func (s *binaryPagerSource) dataLineCount() int {
	if s == nil || s.bytesPerLine <= 0 || s.totalBytes <= 0 {
		return 0
	}
	return int((s.totalBytes + int64(s.bytesPerLine) - 1) / int64(s.bytesPerLine))
}

func (s *binaryPagerSource) Line(idx int) string {
	if s == nil {
		return ""
	}
	if idx < 0 || idx >= s.dataLineCount() {
		return ""
	}
	line, _ := s.lineForDataIndex(idx)
	return line
}

func (s *binaryPagerSource) lineForDataIndex(idx int) (string, error) {
	chunkLines := s.linesPerChunk()
	if chunkLines <= 0 {
		chunkLines = 1
	}
	chunkIndex := idx / chunkLines
	lineOffset := idx % chunkLines
	chunk, err := s.loadChunk(chunkIndex)
	if err != nil {
		return fmt.Sprintf("(error reading file: %v)", err), err
	}
	if chunk == nil || lineOffset >= len(chunk.lines) {
		return "", nil
	}
	return chunk.lines[lineOffset], nil
}

func (s *binaryPagerSource) linesPerChunk() int {
	if s.chunkSize <= 0 || s.bytesPerLine <= 0 {
		return 1
	}
	return s.chunkSize / s.bytesPerLine
}

func (s *binaryPagerSource) loadChunk(index int) (*binaryChunk, error) {
	if chunk, ok := s.cache[index]; ok {
		s.touchChunk(index)
		return chunk, nil
	}
	if s.file == nil {
		file, err := os.Open(s.path)
		if err != nil {
			return nil, err
		}
		s.file = file
	}

	buf := make([]byte, s.chunkSize)
	offset := int64(index) * int64(s.chunkSize)
	n, err := s.file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	buf = buf[:n]
	lines := make([]string, 0, (n+s.bytesPerLine-1)/s.bytesPerLine)
	for i := 0; i < n; i += s.bytesPerLine {
		end := i + s.bytesPerLine
		if end > n {
			end = n
		}
		absOffset := int(offset) + i
		lines = append(lines, formatHexLine(absOffset, buf[i:end], s.bytesPerLine))
	}
	chunk := &binaryChunk{
		index: index,
		lines: lines,
	}
	s.addChunk(index, chunk)
	return chunk, nil
}

func (s *binaryPagerSource) addChunk(index int, chunk *binaryChunk) {
	if s.cache == nil {
		s.cache = make(map[int]*binaryChunk)
	}
	s.cache[index] = chunk
	s.touchChunk(index)
	if len(s.cache) > s.maxChunks {
		evict := s.cacheOrder[0]
		s.cacheOrder = s.cacheOrder[1:]
		delete(s.cache, evict)
	}
}

func (s *binaryPagerSource) touchChunk(index int) {
	for i, v := range s.cacheOrder {
		if v == index {
			s.cacheOrder = append(s.cacheOrder[:i], s.cacheOrder[i+1:]...)
			break
		}
	}
	s.cacheOrder = append(s.cacheOrder, index)
}
