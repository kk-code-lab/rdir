package pager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"github.com/mattn/go-runewidth"
)

func fastIndex(haystack, needle []byte) int {
	return bytes.Index(haystack, needle)
}

func literalMatchSpans(line string, needle []byte, remaining int, caseInsensitive bool) []textSpan {
	if len(needle) == 0 || remaining == 0 || line == "" {
		return nil
	}
	plain := stripANSICodes(line)
	if plain == "" {
		return nil
	}

	if caseInsensitive {
		return matchCaseInsensitive(plain, string(needle), remaining)
	}

	spans := []textSpan{}
	haystack := []byte(plain)
	searchFrom := 0
	for remaining < 0 || len(spans) < remaining {
		idx := fastIndex(haystack[searchFrom:], needle)
		if idx == -1 {
			break
		}
		start := searchFrom + idx
		end := start + len(needle)
		startCol := ansiDisplayWidth(plain[:start])
		endCol := startCol + ansiDisplayWidth(plain[start:end])
		spans = append(spans, textSpan{start: startCol, end: endCol})
		searchFrom = end
	}
	return spans
}

func shiftAndClipSpans(spans []textSpan, drop int, widthLimit int) []textSpan {
	if len(spans) == 0 {
		return nil
	}
	if drop <= 0 && widthLimit <= 0 {
		return spans
	}

	adjusted := make([]textSpan, 0, len(spans))
	for _, sp := range spans {
		if adj, ok := adjustSpan(sp, drop, widthLimit); ok {
			adjusted = append(adjusted, adj)
		}
	}
	if len(adjusted) == 0 {
		return nil
	}
	return adjusted
}

func adjustSpan(span textSpan, drop int, widthLimit int) (textSpan, bool) {
	start := span.start - drop
	end := span.end - drop
	if end <= 0 {
		return textSpan{}, false
	}
	if widthLimit > 0 && start >= widthLimit {
		return textSpan{}, false
	}
	if start < 0 {
		start = 0
	}
	if widthLimit > 0 && end > widthLimit {
		end = widthLimit
	}
	if end <= start {
		return textSpan{}, false
	}
	return textSpan{start: start, end: end}, true
}

func matchCaseInsensitive(plain string, needle string, remaining int) []textSpan {
	if needle == "" || remaining == 0 || plain == "" {
		return nil
	}
	needleLower := strings.ToLower(needle)
	lower := strings.ToLower(plain)

	spans := []textSpan{}
	if len(lower) == len(plain) {
		haystack := []byte(lower)
		needleB := []byte(needleLower)
		searchFrom := 0
		for remaining < 0 || len(spans) < remaining {
			idx := fastIndex(haystack[searchFrom:], needleB)
			if idx == -1 {
				break
			}
			start := searchFrom + idx
			end := start + len(needleB)
			startCol := ansiDisplayWidth(plain[:start])
			endCol := startCol + ansiDisplayWidth(plain[start:end])
			spans = append(spans, textSpan{start: startCol, end: endCol})
			searchFrom = end
		}
		return spans
	}

	for i := 0; i < len(plain); {
		if remaining > 0 && len(spans) >= remaining {
			break
		}
		if matchesAtFolded(plain, i, needleLower) {
			startCol := ansiDisplayWidth(plain[:i])
			endIdx := advanceBytesForRunes(plain, i, utf8.RuneCountInString(needleLower))
			endCol := startCol + ansiDisplayWidth(plain[i:endIdx])
			spans = append(spans, textSpan{start: startCol, end: endCol})
			i = endIdx
			continue
		}
		_, size := utf8.DecodeRuneInString(plain[i:])
		if size <= 0 {
			size = 1
		}
		i += size
	}
	return spans
}

func matchesAtFolded(haystack string, start int, needleLower string) bool {
	hIndex := start
	for _, nr := range needleLower {
		if hIndex >= len(haystack) {
			return false
		}
		hr, size := utf8.DecodeRuneInString(haystack[hIndex:])
		if size <= 0 {
			return false
		}
		if unicode.ToLower(hr) != nr {
			return false
		}
		hIndex += size
	}
	return true
}

func advanceBytesForRunes(s string, start int, runeCount int) int {
	i := start
	for count := 0; i < len(s) && count < runeCount; count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			size = 1
		}
		i += size
	}
	return i
}

func smartCaseInsensitive(query string) bool {
	if query == "" {
		return true
	}
	for _, r := range query {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func (p *PreviewPager) visibleHighlights(lineIdx int, drop int, widthLimit int) ([]textSpan, *textSpan) {
	if p == nil || len(p.searchHighlights) == 0 {
		return nil, nil
	}
	spans, ok := p.searchHighlights[lineIdx]
	if !ok || len(spans) == 0 {
		return nil, nil
	}
	adjusted := shiftAndClipSpans(spans, drop, widthLimit)

	var focus *textSpan
	if hit := p.focusedHit(); hit != nil && hit.line == lineIdx {
		if sp, ok := adjustSpan(hit.span, drop, widthLimit); ok {
			focus = &sp
		}
	}
	return adjusted, focus
}

func hexSpanForByte(byteIdx int, bytesPerLine int) textSpan {
	if byteIdx < 0 {
		byteIdx = 0
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	if byteIdx >= bytesPerLine {
		byteIdx = bytesPerLine - 1
	}
	const hexOffsetCol = 10 // "00000000  "
	col := hexOffsetCol + byteIdx*3
	if byteIdx > 7 {
		col++
	}
	return textSpan{start: col, end: col + 2}
}

func parseBinaryNeedle(query string) ([]byte, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, ":") {
		hex := strings.TrimSpace(trimmed[1:])
		hex = strings.ReplaceAll(hex, " ", "")
		if len(hex)%2 != 0 {
			return nil, errors.New("hex pattern must have even length")
		}
		if len(hex) == 0 {
			return nil, nil
		}
		buf := make([]byte, len(hex)/2)
		for i := 0; i < len(hex); i += 2 {
			var b byte
			_, err := fmt.Sscanf(hex[i:i+2], "%02X", &b)
			if err != nil {
				return nil, errors.New("invalid hex pattern")
			}
			buf[i/2] = b
		}
		return buf, nil
	}
	return []byte(trimmed), nil
}

func applySearchHighlights(text string, spans []textSpan, focus *textSpan) string {
	if text == "" || len(spans) == 0 {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text) + len(spans)*(len(searchHighlightOn)+len(searchHighlightOff)))

	spanIdx := 0
	current := spans[spanIdx]
	currentStyle := ""
	col := 0
	active := false
	activeSGR := ""

	for i := 0; i < len(text); {
		if spanIdx >= len(spans) {
			builder.WriteString(text[i:])
			break
		}
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			end := i + 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			activeSGR = text[i:end]
			builder.WriteString(text[i:end])
			i = end
			continue
		}

		ru, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
			ru = rune(text[i])
		}
		width := runewidth.RuneWidth(ru)
		if width <= 0 {
			width = 1
		}

		if !active && col >= current.start {
			currentStyle = highlightStyleForSpan(current, focus)
			builder.WriteString(currentStyle)
			active = true
		}

		builder.WriteString(text[i : i+size])
		col += width

		for active && col >= current.end {
			builder.WriteString(highlightOffForStyle(currentStyle))
			if activeSGR != "" {
				builder.WriteString(activeSGR)
			}
			active = false
			spanIdx++
			if spanIdx >= len(spans) {
				current = textSpan{}
				currentStyle = ""
				break
			}
			current = spans[spanIdx]
			if col >= current.start && col < current.end {
				currentStyle = highlightStyleForSpan(current, focus)
				builder.WriteString(currentStyle)
				active = true
			}
		}

		i += size
	}

	if active {
		builder.WriteString(highlightOffForStyle(currentStyle))
		if activeSGR != "" {
			builder.WriteString(activeSGR)
		}
	}
	return builder.String()
}

func highlightStyleForSpan(span textSpan, focus *textSpan) string {
	if focus != nil && span.start == focus.start && span.end == focus.end {
		return searchHighlightFocusOn
	}
	return searchHighlightOn
}

func highlightOffForStyle(style string) string {
	if style == searchHighlightFocusOn {
		return searchHighlightFocusOff
	}
	return searchHighlightOff
}

func (p *PreviewPager) enterSearchMode() {
	if p == nil {
		return
	}
	p.searchMode = true
	if len(p.searchQuery) > 0 {
		p.searchInput = append([]rune(nil), []rune(p.searchQuery)...)
	} else {
		p.searchInput = nil
	}
	p.searchErr = nil
}

func (p *PreviewPager) exitSearchMode() {
	p.searchMode = false
	p.searchInput = nil
	p.stopSearchTimer()
}

func (p *PreviewPager) cancelSearch() {
	p.searchMode = false
	p.searchInput = nil
	p.stopSearchTimer()
	p.searchQuery = ""
	p.clearSearchResults()
}

func (p *PreviewPager) appendSearchRune(ch rune) {
	if ch == 0 {
		return
	}
	p.searchInput = append(p.searchInput, ch)
	p.onSearchInputChanged()
}

func (p *PreviewPager) backspaceSearch() {
	if len(p.searchInput) == 0 {
		return
	}
	p.searchInput = p.searchInput[:len(p.searchInput)-1]
	p.onSearchInputChanged()
}

func (p *PreviewPager) clearSearchResults() {
	p.searchHits = nil
	p.searchHighlights = nil
	p.searchCursor = -1
	p.searchLimited = false
	p.searchErr = nil
	p.searchFocused = false
}

func (p *PreviewPager) onSearchInputChanged() {
	p.clearSearchResults()
	p.searchQuery = ""
	if len(p.searchInput) == 0 {
		p.stopSearchTimer()
		return
	}
	p.scheduleSearchRun()
}

func (p *PreviewPager) scheduleSearchRun() {
	p.stopSearchTimer()
	delay := searchDebounceDelay
	if delay <= 0 {
		p.executeSearch(string(p.searchInput))
		return
	}
	p.searchTimer = time.NewTimer(delay)
}

func (p *PreviewPager) stopSearchTimer() {
	if p.searchTimer == nil {
		return
	}
	if !p.searchTimer.Stop() {
		select {
		case <-p.searchTimer.C:
		default:
		}
	}
	p.searchTimer = nil
}

func (p *PreviewPager) searchTimerC() <-chan time.Time {
	if p.searchTimer == nil {
		return nil
	}
	return p.searchTimer.C
}

func (p *PreviewPager) runPendingSearch() {
	p.stopSearchTimer()
	p.executeSearch(string(p.searchInput))
}

func (p *PreviewPager) finalizeSearchInput() {
	if p.searchTimer != nil {
		p.runPendingSearch()
		return
	}
	current := string(p.searchInput)
	if current != p.searchQuery {
		p.executeSearch(current)
	}
}

func (p *PreviewPager) executeSearch(query string) {
	p.clearSearchResults()
	p.searchQuery = query
	p.searchFocused = false
	if strings.TrimSpace(query) == "" {
		return
	}

	var (
		hits       []searchHit
		highlights map[int][]textSpan
		limited    bool
		err        error
	)

	if p.binaryMode {
		hits, highlights, limited, err = p.collectBinarySearchMatches(query)
	} else {
		hits, highlights, limited, err = p.collectSearchMatches(query)
	}
	if err != nil {
		p.searchErr = err
		return
	}
	p.searchHits = hits
	p.searchHighlights = highlights
	p.searchLimited = limited
	p.setInitialSearchCursor()
}

func (p *PreviewPager) collectSearchMatches(query string) ([]searchHit, map[int][]textSpan, bool, error) {
	needle := []byte(query)
	if len(needle) == 0 {
		return nil, nil, false, nil
	}
	if p.rawTextSource != nil {
		return p.searchStreaming(needle)
	}
	return p.searchStatic(needle)
}

func (p *PreviewPager) collectBinarySearchMatches(query string) ([]searchHit, map[int][]textSpan, bool, error) {
	if p == nil || p.state == nil || p.binarySource == nil {
		return nil, nil, false, errors.New("binary source unavailable")
	}

	needle, err := parseBinaryNeedle(query)
	if err != nil {
		return nil, nil, false, err
	}
	if len(needle) == 0 {
		return nil, nil, false, nil
	}

	bytesPerLine := p.binarySource.bytesPerLine
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	totalBytes := p.binarySource.totalBytes
	if totalBytes <= 0 && p.state.PreviewData != nil {
		totalBytes = p.state.PreviewData.Size
	}
	if totalBytes <= 0 {
		return nil, nil, false, nil
	}

	maxLines := searchMaxLines
	totalLines := int((totalBytes + int64(bytesPerLine) - 1) / int64(bytesPerLine))
	if totalLines < maxLines {
		maxLines = totalLines
	}
	bytesToScan := int64(maxLines) * int64(bytesPerLine)
	if bytesToScan > totalBytes {
		bytesToScan = totalBytes
	}

	bufSize := p.binarySource.chunkSize
	if bufSize < bytesPerLine {
		bufSize = bytesPerLine * 64
	}
	overlap := len(needle) - 1
	window := make([]byte, bufSize+overlap)

	file := p.binarySource.file
	if file == nil {
		f, openErr := os.Open(p.binarySource.path)
		if openErr != nil {
			return nil, nil, false, openErr
		}
		file = f
		defer func() { _ = file.Close() }()
	}

	hits := []searchHit{}
	highlights := make(map[int][]textSpan)
	limited := totalBytes > bytesToScan

	var tail []byte
	for offset := int64(0); offset < bytesToScan && len(hits) < searchMaxHits; {
		readSize := bufSize
		if int64(readSize) > bytesToScan-offset {
			readSize = int(bytesToScan - offset)
		}
		n, err := file.ReadAt(window[:readSize], offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return hits, highlights, limited, err
		}
		if n == 0 {
			break
		}
		chunk := append(tail, window[:n]...)

		searchFrom := 0
		for len(hits) < searchMaxHits {
			idx := bytes.Index(chunk[searchFrom:], needle)
			if idx == -1 {
				break
			}
			abs := offset - int64(len(tail)) + int64(searchFrom+idx)
			line := int(abs / int64(bytesPerLine))
			col := int(abs % int64(bytesPerLine))
			sp := hexSpanForByte(col, bytesPerLine)
			highlights[line] = append(highlights[line], sp)
			hits = append(hits, searchHit{line: line, span: sp})
			searchFrom += idx + 1
		}

		if len(hits) >= searchMaxHits {
			limited = true
			break
		}

		// carry overlap
		if overlap > n {
			overlap = n
		}
		tail = append([]byte(nil), chunk[len(chunk)-overlap:]...)
		offset += int64(n)
	}

	return hits, highlights, limited, nil
}

func (p *PreviewPager) searchStreaming(needle []byte) ([]searchHit, map[int][]textSpan, bool, error) {
	src := p.rawTextSource
	if src == nil {
		return nil, nil, false, errors.New("streaming source unavailable")
	}
	caseInsensitive := smartCaseInsensitive(string(needle))
	hits := []searchHit{}
	highlights := make(map[int][]textSpan)
	linesScanned := 0
	limited := false

	for i := 0; i < searchMaxLines; i++ {
		if err := src.EnsureLine(i); err != nil && !errors.Is(err, io.EOF) {
			return hits, highlights, limited, err
		}
		if i >= src.LineCount() {
			break
		}
		line := textutil.SanitizeTerminalText(src.Line(i))
		spans := literalMatchSpans(line, needle, searchMaxHits-len(hits), caseInsensitive)
		if len(spans) > 0 {
			highlights[i] = spans
			for _, sp := range spans {
				hits = append(hits, searchHit{line: i, span: sp})
				if len(hits) >= searchMaxHits {
					limited = true
					if !src.FullyLoaded() || i+1 < src.LineCount() {
						limited = true
					}
					return hits, highlights, limited, nil
				}
			}
		}
		linesScanned++
	}

	total := src.LineCount()
	if linesScanned >= searchMaxLines && total > linesScanned {
		limited = true
	}
	if !src.FullyLoaded() && total > linesScanned {
		limited = true
	}

	return hits, highlights, limited, nil
}

func (p *PreviewPager) searchStatic(needle []byte) ([]searchHit, map[int][]textSpan, bool, error) {
	total := p.lineCount()
	limit := total
	limited := false
	if limit > searchMaxLines {
		limit = searchMaxLines
		limited = true
	}

	caseInsensitive := smartCaseInsensitive(string(needle))
	hits := []searchHit{}
	highlights := make(map[int][]textSpan)
	remaining := searchMaxHits

	for i := 0; i < limit; i++ {
		line := p.lineAt(i)
		if remaining == 0 {
			limited = true
			break
		}
		spans := literalMatchSpans(line, needle, remaining, caseInsensitive)
		if len(spans) == 0 {
			continue
		}
		highlights[i] = spans
		for _, sp := range spans {
			hits = append(hits, searchHit{line: i, span: sp})
			remaining--
			if remaining == 0 {
				limited = true
				break
			}
		}
	}

	return hits, highlights, limited, nil
}

func (p *PreviewPager) moveSearchCursor(delta int) {
	if len(p.searchHits) == 0 {
		return
	}
	if p.searchCursor < 0 || p.searchCursor >= len(p.searchHits) {
		p.searchCursor = 0
	}
	currentHit := p.searchHits[p.searchCursor]
	if !p.searchFocused {
		if !p.hitVisible(currentHit) {
			p.focusSearchHit(p.searchCursor)
			return
		}
		p.searchFocused = true
	}
	p.searchCursor = (p.searchCursor + delta) % len(p.searchHits)
	if p.searchCursor < 0 {
		p.searchCursor += len(p.searchHits)
	}
	p.focusSearchHit(p.searchCursor)
}

func (p *PreviewPager) focusSearchHit(idx int) {
	if p == nil || p.state == nil || idx < 0 || idx >= len(p.searchHits) {
		return
	}
	hit := p.searchHits[idx]
	p.searchFocused = true
	if p.rawTextSource != nil {
		_ = p.rawTextSource.EnsureLine(hit.line)
	}

	headerRows := len(p.headerLines())
	if headerRows >= p.height {
		headerRows = p.height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}
	contentRows := p.height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	totalLines := p.lineCount()
	if !p.wrapEnabled || p.width <= 0 {
		target := hit.line - contentRows/2
		if target < 0 {
			target = 0
		}
		p.state.PreviewScrollOffset = target
		p.state.PreviewWrapOffset = 0
		p.clampScroll(totalLines, contentRows)
		return
	}

	p.ensureRowMetrics()
	hitRowOffset := 0
	if p.width > 0 && hit.span.start > 0 {
		hitRowOffset = hit.span.start / p.width
	}
	baseRow := 0
	if hit.line >= 0 && hit.line < len(p.rowPrefix) {
		baseRow = p.rowPrefix[hit.line]
	}
	targetRow := baseRow + hitRowOffset
	startRow := targetRow - contentRows/2
	lineIdx, wrapOffset := p.positionFromRow(startRow)
	p.state.PreviewScrollOffset = lineIdx
	p.state.PreviewWrapOffset = wrapOffset
	p.clampScroll(totalLines, contentRows)
}

func (p *PreviewPager) hitRowRange(hit searchHit) (int, int) {
	if p == nil {
		return hit.line, hit.line
	}
	if !p.wrapEnabled || p.width <= 0 {
		return hit.line, hit.line
	}
	p.ensureRowMetrics()
	if hit.line < 0 || hit.line >= len(p.rowPrefix) {
		return hit.line, hit.line
	}
	base := p.rowPrefix[hit.line]
	width := p.width
	if width <= 0 {
		width = 1
	}
	start := base + hit.span.start/width
	end := base + (hit.span.end-1)/width
	if end < start {
		end = start
	}
	return start, end
}

func (p *PreviewPager) hitVisible(hit searchHit) bool {
	if p == nil || p.state == nil {
		return false
	}

	headerRows := len(p.headerLines())
	if headerRows >= p.height {
		headerRows = p.height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}
	contentRows := p.height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	if p.wrapEnabled && p.width > 0 {
		p.ensureRowMetrics()
		startRow := p.currentRowNumber()
		endRow := startRow + contentRows - 1
		hitStart, hitEnd := p.hitRowRange(hit)
		if hitStart > endRow || hitEnd < startRow {
			return false
		}
		return true
	}

	startLine := p.state.PreviewScrollOffset
	if startLine < 0 {
		startLine = 0
	}
	endLine := startLine + contentRows - 1
	total := p.lineCount()
	if total <= 0 {
		return false
	}
	if endLine >= total {
		endLine = total - 1
	}
	return hit.line >= startLine && hit.line <= endLine
}

func (p *PreviewPager) setInitialSearchCursor() {
	if len(p.searchHits) == 0 {
		p.searchCursor = -1
		p.searchFocused = false
		return
	}

	origWidth := p.width
	if p.wrapEnabled && p.width <= 0 {
		p.width = 80
		defer func() {
			p.width = origWidth
		}()
	}
	if p.wrapEnabled && p.width > 0 {
		p.ensureRowMetrics()
	}

	currentRow := p.currentRowNumber()
	best := -1
	for i, hit := range p.searchHits {
		start, end := p.hitRowRange(hit)
		if currentRow >= start && currentRow <= end {
			best = i
			break
		}
		if start >= currentRow && best == -1 {
			best = i
		}
	}
	if best == -1 {
		best = 0
	}
	p.searchCursor = best
	p.searchFocused = false
}

func (p *PreviewPager) focusedHit() *searchHit {
	if p == nil || len(p.searchHits) == 0 {
		return nil
	}
	if p.searchCursor < 0 || p.searchCursor >= len(p.searchHits) {
		return nil
	}
	return &p.searchHits[p.searchCursor]
}
