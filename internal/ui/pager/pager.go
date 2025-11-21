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

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"github.com/mattn/go-runewidth"
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
	clipboardWarnBytes      = int64(16 * 1024 * 1024)
	clipboardHardLimitBytes = int64(128 * 1024 * 1024)
	shiftScrollLines        = 10
	searchHighlightOn       = "\x1b[38;5;16;48;5;255m"
	searchHighlightOff      = "\x1b[39;49m"
	searchHighlightFocusOn  = "\x1b[38;5;16;48;5;178m"
	searchHighlightFocusOff = "\x1b[39;49m"
	searchDebounceDelay     = 140 * time.Millisecond
)

var (
	searchMaxHits  = 10000
	searchMaxLines = 20000
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
	line int
	span textSpan
}

var termGetSize = term.GetSize

type PreviewPager struct {
	state            *statepkg.AppState
	editorCmd        []string
	reducer          *statepkg.StateReducer
	input            *os.File
	outputFile       *os.File
	output           io.Writer
	reader           *bufio.Reader
	writer           *bufio.Writer
	restoreTerm      *term.State
	stopKeyReader    func()
	width            int
	height           int
	wrapEnabled      bool
	lines            []string
	lineWidths       []int
	rawLines         []string
	rawLineWidths    []int
	rawSanitized     []string
	rawSanitizedWid  []int
	formattedLines   []string
	formattedWidths  []int
	formattedRules   []bool
	formattedStyles  []string
	rowSpans         []int
	rowPrefix        []int
	rowMetricsWidth  int
	charCount        int
	binaryMode       bool
	binarySource     *binaryPagerSource
	rawTextSource    *textPagerSource
	preloadLines     int
	showInfo         bool
	showHelp         bool
	showFormatted    bool
	statusMessage    string
	statusStyle      string
	statusExpiry     time.Time
	statusTimer      *time.Timer
	lastErr          error
	restartKeys      bool
	clipboardCmd     []string
	clipboardFunc    func(string) error
	searchMode       bool
	searchInput      []rune
	searchQuery      string
	searchHits       []searchHit
	searchCursor     int
	searchHighlights map[int][]textSpan
	searchLimited    bool
	searchErr        error
	searchTimer      *time.Timer
	searchFocused    bool
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
			updated := false
			if p.statusTimer != nil {
				select {
				case <-p.statusTimer.C:
					p.clearStatusMessage()
					updated = true
				default:
				}
			}
			if ch := p.searchTimerC(); ch != nil {
				select {
				case <-ch:
					p.runPendingSearch()
					updated = true
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
			needsRender = true || updated
			continue
		}

		select {
		case <-resizeEvents:
			needsRender = true
		case event := <-keyEvents:
			if done := p.handleKey(event); done {
				return p.lastErr
			}
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
	if p.tryUpdateSizeFromFile(p.input) {
		return
	}
	if p.outputFile != nil && p.outputFile != p.input {
		_ = p.tryUpdateSizeFromFile(p.outputFile)
	}
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

	contentRows := p.height - headerRows - 1 // leave space for status
	if contentRows < 1 {
		contentRows = 1
	}
	contentRowLimit := p.height - 1
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
		displayText := text
		currentSkip := skipRows
		if p.wrapEnabled && currentSkip > 0 {
			displayText = p.trimWrappedPrefix(displayText, currentSkip)
		}
		if !p.wrapEnabled && p.width > 0 {
			displayText = truncateToWidth(displayText, p.width)
		}
		if !p.binaryMode {
			dropCols := 0
			if p.wrapEnabled && p.width > 0 && currentSkip > 0 {
				dropCols = currentSkip * p.width
			}
			widthLimit := 0
			if !p.wrapEnabled && p.width > 0 {
				widthLimit = p.width
			}
			if spans, focus := p.visibleHighlights(i, dropCols, widthLimit); len(spans) > 0 {
				displayText = applySearchHighlights(displayText, spans, focus)
			}
		}
		p.drawRow(row, displayText, false)
		rowsUsed := p.rowSpanForIndex(i)
		if currentSkip > 0 {
			rowsUsed -= currentSkip
			if rowsUsed < 1 {
				rowsUsed = 1
			}
		}
		row += rowsUsed
		skipRows = 0
	}

	for row <= contentRowLimit {
		p.drawRow(row, "", false)
		row++
	}

	status := p.statusLine(totalLines, contentRows, p.totalCharCount())
	p.drawStatus(status)

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
	p.writeString("\x1b[2K")
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
	p.writeString("\x1b[2K")

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
		if !p.wrapEnabled {
			p.state.PreviewWrapOffset = 0
		}
		p.rowMetricsWidth = 0
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
		if p.binaryMode {
			p.setStatusMessage("search unavailable in binary preview", statusErrorStyle)
			break
		}
		p.enterSearchMode()
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
	}

	p.clampScroll(totalLines, contentRows)
	return false
}

func (p *PreviewPager) handleSearchModeEvent(ev keyEvent) {
	switch ev.kind {
	case keyEscape:
		p.cancelSearch()
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

func (p *PreviewPager) statusLine(totalLines, visible, charCount int) string {
	lineApprox := p.isLineCountApprox()
	charApprox := p.isCharCountApprox()
	kind := p.contentKind()

	segments := []string{p.positionSegment(totalLines, visible, lineApprox)}
	if count := p.countSegment(kind, charCount, charApprox); count != "" {
		segments = append(segments, count)
	}
	segments = append(segments, p.statusBadges(kind)...)
	if search := p.searchStatusSegment(); search != "" {
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
	return badges
}

func (p *PreviewPager) searchStatusSegment() string {
	if p == nil || p.binaryMode {
		return ""
	}
	displayRaw := p.searchQuery
	if p.searchMode {
		displayRaw = string(p.searchInput)
	}
	trimmed := strings.TrimSpace(displayRaw)
	if trimmed == "" {
		if p.searchMode {
			return "/_"
		}
		return ""
	}

	safeQuery := textutil.SanitizeTerminalText(trimmed)
	segment := "/" + safeQuery
	if p.searchMode {
		segment += "_"
	}

	activeQuery := strings.TrimSpace(p.searchQuery)
	useResults := activeQuery != "" && strings.EqualFold(activeQuery, trimmed)
	if !useResults {
		return segment
	}

	if p.searchErr != nil {
		return segment + " !"
	}

	total := len(p.searchHits)
	if total == 0 {
		if p.searchLimited {
			return segment + " 0/0+"
		}
		return segment + " 0/0"
	}

	cursor := p.searchCursor
	if cursor < 0 || cursor >= total {
		cursor = 0
	}
	segment += fmt.Sprintf(" %d/%d", cursor+1, total)
	if p.searchLimited {
		segment += "+"
	}
	return segment
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
			helpEntry{keys: "C", desc: "Copy entire file"},
		)
	}
	if p.canOpenEditor() {
		actions = append(actions, helpEntry{keys: "e", desc: "Open in editor"})
	}
	actions = append(actions, helpEntry{keys: "Ctrl+C", desc: "Quit immediately"})

	exit := []helpEntry{
		{keys: "← / q / x / Esc", desc: "Exit pager"},
	}

	search := []helpEntry{}
	if !p.binaryMode {
		search = append(search,
			helpEntry{keys: "/", desc: "Enter search"},
			helpEntry{keys: "n / N", desc: "Jump to next/prev hit"},
		)
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
	segments := []string{"? help"}
	if !p.binaryMode {
		segments = append(segments, "/ search")
	}
	return segments
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
	lines := p.visibleContentLines()
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

	if p.clipboardFunc != nil {
		var builder strings.Builder
		if err := p.writeAllLines(&builder); err != nil {
			return "", "", err
		}
		if err := p.clipboardFunc(builder.String()); err != nil {
			return "", "", err
		}
	} else {
		if err := p.streamAllLinesToClipboard(); err != nil {
			return "", "", err
		}
	}

	msg := "copied all"
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
		for index < len(text) {
			ru, size := utf8.DecodeRuneInString(text[index:])
			if size <= 0 {
				index++
				break
			}
			w := runewidth.RuneWidth(ru)
			if w <= 0 {
				w = 1
			}
			if consumed+w > width {
				if consumed == 0 {
					index += size
				}
				break
			}
			consumed += w
			index += size
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
		return []string{"Preview unavailable"}
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
	if p.wrapEnabled {
		p.writeString("\x1b[?7h")
	} else {
		p.writeString("\x1b[?7l")
	}
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
		ru, size := utf8.DecodeRuneInString(text[index:])
		if ru == utf8.RuneError && size == 1 {
			consumed++
			index++
			continue
		}
		w := runewidth.RuneWidth(ru)
		if w < 1 {
			w = 1
		}
		consumed += w
		index += size
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
		source, err := newBinaryPagerSource(filePath, preview.BinaryInfo.TotalBytes)
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
	keySearchNext
	keySearchPrev
	keyEnter
	keyBackspace
	keyRune
)

type keyEvent struct {
	kind keyKind
	ch   rune
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
	case 'n':
		return keyEvent{kind: keySearchNext, ch: rune(b)}, nil
	case 'N':
		return keyEvent{kind: keySearchPrev, ch: rune(b)}, nil
	case ' ':
		return keyEvent{kind: keySpace, ch: rune(b)}, nil
	case 'b', 'B':
		return keyEvent{kind: keyPageUp, ch: rune(b)}, nil
	case 'g':
		return keyEvent{kind: keyHome, ch: rune(b)}, nil
	case 'G':
		return keyEvent{kind: keyEnd, ch: rune(b)}, nil
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
	params, modifier := parseCSIParameters(string(seq[:len(seq)-1]))

	switch final {
	case 'A':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftUp}, nil
		}
		return keyEvent{kind: keyUp}, nil
	case 'B':
		if hasShiftModifier(modifier) {
			return keyEvent{kind: keyShiftDown}, nil
		}
		return keyEvent{kind: keyDown}, nil
	case 'C':
		return keyEvent{kind: keyRight}, nil
	case 'D':
		return keyEvent{kind: keyLeft}, nil
	case 'H':
		return keyEvent{kind: keyHome}, nil
	case 'F':
		return keyEvent{kind: keyEnd}, nil
	case '~':
		switch params {
		case "3":
			return keyEvent{kind: keyUnknown}, nil
		case "5":
			return keyEvent{kind: keyPageUp}, nil
		case "6":
			return keyEvent{kind: keyPageDown}, nil
		case "1":
			return keyEvent{kind: keyHome}, nil
		case "4":
			return keyEvent{kind: keyEnd}, nil
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
		return "\x1b[2m"
	case statepkg.TextStyleLink:
		return "\x1b[4m"
	case statepkg.TextStyleRule:
		return "\x1b[2m"
	default:
		return ""
	}
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
		if ru == utf8.RuneError && size == 1 {
			width++
			i++
			continue
		}
		w := runewidth.RuneWidth(ru)
		if w <= 0 {
			w = 1
		}
		width += w
		i += size
	}
	return width
}

func ansiTruncate(text string, width int, withEllipsis bool) (string, bool) {
	if width <= 0 {
		return "", ansiDisplayWidth(text) > 0
	}
	const ellipsisRune = '…'
	ellipsisWidth := runewidth.RuneWidth(ellipsisRune)
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	target := width
	if withEllipsis {
		if width <= ellipsisWidth {
			return string(ellipsisRune), true
		}
		target = width - ellipsisWidth
	}

	var b strings.Builder
	consumed := 0
	for i := 0; i < len(text) && consumed < target; {
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) && text[j] != 'm' {
				j++
			}
			if j < len(text) {
				j++
			}
			b.WriteString(text[i:j])
			i = j
			continue
		}
		ru, size := utf8.DecodeRuneInString(text[i:])
		if ru == utf8.RuneError && size == 1 {
			if consumed+1 > target {
				break
			}
			b.WriteByte(text[i])
			consumed++
			i++
			continue
		}
		w := runewidth.RuneWidth(ru)
		if w <= 0 {
			w = 1
		}
		if consumed+w > target {
			break
		}
		b.WriteRune(ru)
		consumed += w
		i += size
	}

	truncated := consumed < ansiDisplayWidth(text)
	if withEllipsis && truncated {
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

func formatHexLine(offset int, chunk []byte) string {
	var builder strings.Builder
	builder.Grow(80)
	fmt.Fprintf(&builder, "%08X  ", offset)

	for i := 0; i < binaryPreviewLineWidth; i++ {
		if i < len(chunk) {
			fmt.Fprintf(&builder, "%02X ", chunk[i])
		} else {
			builder.WriteString("   ")
		}
		if i == 7 {
			builder.WriteString(" ")
		}
	}

	builder.WriteString(" |")
	for i := 0; i < len(chunk); i++ {
		builder.WriteByte(printableASCII(chunk[i]))
	}
	for i := len(chunk); i < binaryPreviewLineWidth; i++ {
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

func newBinaryPagerSource(path string, totalBytes int64) (*binaryPagerSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	source := &binaryPagerSource{
		path:         path,
		totalBytes:   totalBytes,
		bytesPerLine: binaryPreviewLineWidth,
		chunkSize:    binaryPagerChunkSize,
		maxChunks:    binaryPagerMaxChunks,
		file:         file,
		cache:        make(map[int]*binaryChunk),
	}
	if source.chunkSize < source.bytesPerLine {
		source.chunkSize = source.bytesPerLine
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
		lines = append(lines, formatHexLine(absOffset, buf[i:end]))
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
