package pager

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const (
	binaryPreviewLineWidth = 16
	binaryPagerChunkSize   = 64 * 1024
	binaryPagerMaxChunks   = 8
)

var termGetSize = term.GetSize

type PreviewPager struct {
	state           *statepkg.AppState
	input           *os.File
	outputFile      *os.File
	output          io.Writer
	reader          *bufio.Reader
	writer          *bufio.Writer
	restoreTerm     *term.State
	width           int
	height          int
	wrapEnabled     bool
	lines           []string
	lineWidths      []int
	rowSpans        []int
	rowPrefix       []int
	rowMetricsWidth int
	charCount       int
	binaryMode      bool
	binarySource    *binaryPagerSource
}

func NewPreviewPager(state *statepkg.AppState) (*PreviewPager, error) {
	if state == nil || state.PreviewData == nil {
		return nil, errors.New("preview data unavailable")
	}
	pager := &PreviewPager{
		state:       state,
		wrapEnabled: state.PreviewWrap,
	}
	pager.prepareContent()
	return pager, nil
}

func (p *PreviewPager) prepareContent() {
	lines, charCount, binarySource := p.buildContentLines()
	if binarySource != nil {
		p.binaryMode = true
		p.wrapEnabled = false
		p.binarySource = binarySource
		p.lines = nil
		p.lineWidths = nil
		p.charCount = charCount
		return
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	widths := make([]int, len(lines))
	for i, line := range lines {
		widths[i] = displayWidth(line)
	}
	p.lines = lines
	p.lineWidths = widths
	p.charCount = charCount
}

func (p *PreviewPager) Run() error {
	if err := p.initTerminal(); err != nil {
		return err
	}
	defer p.cleanupTerminal()

	p.updateSize()
	p.applyWrapSetting()
	for {
		if err := p.render(); err != nil {
			return err
		}

		event, err := p.readKeyEvent()
		if err != nil {
			return err
		}

		if done := p.handleKey(event); done {
			return nil
		}
	}
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
	if p.input != nil && p.restoreTerm != nil {
		_ = term.Restore(int(p.input.Fd()), p.restoreTerm)
	}
	if p.writer != nil {
		_ = p.writer.Flush()
	}
	p.writeString("\x1b[?25h")
	p.writeString("\x1b[?7h")
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

	p.ensureRowMetrics()

	header := p.headerLines()
	headerRows := len(header) + 1 // + blank separator line
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
		p.drawRow(row, line, true)
		row++
	}

	if row <= p.height-1 {
		p.drawRow(row, "", false)
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

	for i := start; i < totalLines && row <= p.height-1; i++ {
		text := p.lineAt(i)
		currentSkip := skipRows
		if p.wrapEnabled && currentSkip > 0 {
			text = p.trimWrappedPrefix(text, currentSkip)
		}
		p.drawRow(row, text, false)
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

	for row <= p.height-1 {
		p.drawRow(row, "", false)
		row++
	}

	status := p.statusLine(totalLines, contentRows, p.charCount)
	p.drawStatus(status)

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

func (p *PreviewPager) drawStatus(text string) {
	if p.height < 1 {
		return
	}
	p.printf("\x1b[%d;1H", p.height)
	p.writeString("\x1b[2K")
	if len(text) > p.width && p.width > 0 {
		text = truncateToWidth(text, p.width)
	}
	p.printf("\x1b[7m %s \x1b[0m", text)
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
	totalLines := p.lineCount()
	if p.wrapEnabled {
		p.ensureRowMetrics()
	}

	contentRows := p.height - (len(p.headerLines()) + 1) - 1
	if contentRows < 1 {
		contentRows = 1
	}

	switch ev.kind {
	case keyQuit, keyEscape, keyCtrlC, keyLeft:
		return true
	case keyUp:
		if p.wrapEnabled {
			p.scrollRows(totalLines, -1)
		} else {
			p.state.PreviewScrollOffset--
		}
	case keyDown:
		if p.wrapEnabled {
			p.scrollRows(totalLines, 1)
		} else {
			p.state.PreviewScrollOffset++
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
	}

	p.clampScroll(totalLines, contentRows)
	return false
}

func (p *PreviewPager) statusLine(totalLines, visible, charCount int) string {
	wrap := "off"
	if p.wrapEnabled {
		wrap = "on"
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
		return fmt.Sprintf("%d-%d/%d rows (%d lines, %d%%)  %d chars  wrap:%s  ↑↓/PgUp/PgDn scroll  ←/Esc/q exit  w/→ wrap",
			startRow, endRow, totalRows, totalLines, percent, charCount, wrap)
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
	return fmt.Sprintf("%d-%d/%d lines (%d%%)  %d chars  wrap:%s  ↑↓/PgUp/PgDn scroll  ←/Esc/q exit  w/→ wrap",
		start, end, totalLines, percent, charCount, wrap)
}

func (p *PreviewPager) lineCount() int {
	if p.binaryMode {
		if p.binarySource == nil {
			return 0
		}
		return p.binarySource.LineCount()
	}
	return len(p.lines)
}

func (p *PreviewPager) lineAt(idx int) string {
	if p.binaryMode {
		if p.binarySource == nil {
			return ""
		}
		return p.binarySource.Line(idx)
	}
	if idx < 0 || idx >= len(p.lines) {
		return ""
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
	size := formatSize(preview.Size)
	mod := preview.Modified.Format("2006-01-02 15:04:05")
	mode := preview.Mode.String()

	return []string{
		fullPath,
		fmt.Sprintf("%s  %s  %s", mode, size, mod),
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
	if p.binaryMode {
		return displayWidth(p.lineAt(idx))
	}
	if idx < 0 || idx >= len(p.lineWidths) {
		return 0
	}
	return p.lineWidths[idx]
}

func (p *PreviewPager) ensureRowMetrics() {
	if p.binaryMode || !p.wrapEnabled || p.width <= 0 || len(p.lines) == 0 {
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

func (p *PreviewPager) buildContentLines() ([]string, int, *binaryPagerSource) {
	if p.state == nil || p.state.PreviewData == nil {
		return nil, 0, nil
	}

	preview := p.state.PreviewData
	switch {
	case preview.IsDir:
		lines := formatDirectoryPreview(preview)
		return lines, lineCharCount(lines), nil
	case len(preview.TextLines) > 0:
		return preview.TextLines, preview.TextCharCount, nil
	case len(preview.BinaryInfo.Lines) > 0:
		filePath := filepath.Join(p.state.CurrentPath, preview.Name)
		source, err := newBinaryPagerSource(filePath, preview.BinaryInfo.TotalBytes)
		if err == nil {
			return nil, int(preview.BinaryInfo.TotalBytes), source
		}
		lines := append([]string(nil), preview.BinaryInfo.Lines...)
		if len(lines) > 0 {
			lines = lines[1:]
		}
		return lines, lineCharCount(lines), nil
	default:
		lines := []string{"(no preview available)"}
		return lines, lineCharCount(lines), nil
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
)

type keyEvent struct {
	kind keyKind
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
	case 'k', 'K':
		return keyEvent{kind: keyUp}, nil
	case 'j', 'J':
		return keyEvent{kind: keyDown}, nil
	case 'q', 'Q':
		return keyEvent{kind: keyQuit}, nil
	case 'x', 'X':
		return keyEvent{kind: keyQuit}, nil
	case 'w', 'W':
		return keyEvent{kind: keyToggleWrap}, nil
	case ' ':
		return keyEvent{kind: keySpace}, nil
	case 'b', 'B':
		return keyEvent{kind: keyPageUp}, nil
	case 'g':
		return keyEvent{kind: keyHome}, nil
	case 'G':
		return keyEvent{kind: keyEnd}, nil
	case 0x03:
		return keyEvent{kind: keyCtrlC}, nil
	default:
		if b == '\r' || b == '\n' {
			return keyEvent{kind: keySpace}, nil
		}
	}

	if b < utf8.RuneSelf {
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
		if (b >= 'A' && b <= 'Z') || b == '~' {
			break
		}
		if len(seq) > 5 {
			break
		}
	}

	switch seq[len(seq)-1] {
	case 'A':
		return keyEvent{kind: keyUp}, nil
	case 'B':
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
		str := string(seq[:len(seq)-1])
		switch str {
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
	size := formatSize(entry.Size)
	mod := entry.Modified.Format("2006-01-02 15:04:05")
	return fmt.Sprintf(" %s %-20s %12s  %s  %s", icon, entry.Name, size, entry.Mode.String(), mod)
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
	if displayWidth(text) <= width {
		return text
	}

	const ellipsis = "…"
	ellipsisWidth := runewidth.RuneWidth([]rune(ellipsis)[0])
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	if width <= ellipsisWidth {
		return ellipsis
	}

	target := width - ellipsisWidth
	var builder strings.Builder
	current := 0
	for _, ru := range text {
		w := runewidth.RuneWidth(ru)
		if w <= 0 {
			w = 1
		}
		if current+w > target {
			break
		}
		builder.WriteRune(ru)
		current += w
	}
	builder.WriteString(ellipsis)
	return builder.String()
}

func displayWidth(text string) int {
	width := 0
	for _, ru := range text {
		w := runewidth.RuneWidth(ru)
		if w <= 0 {
			w = 1
		}
		width += w
	}
	return width
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
