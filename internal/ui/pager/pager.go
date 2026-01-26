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
	"strings"
	"sync"
	"time"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
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

func (p *PreviewPager) effectiveBinaryFullScan() bool {
	if p == nil || !p.binaryMode {
		return false
	}
	if p.searchMode {
		return p.searchFullScan
	}
	return p.searchQueryFullScan
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

func (p *PreviewPager) applyWrapSetting() {
	if p.output == nil {
		return
	}
	p.writeString("\x1b[?7l")
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
