package pager

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"github.com/rivo/uniseg"
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

func (p *PreviewPager) visibleHighlights(lineIdx int, drop int, widthLimit int) ([]textSpan, []textSpan) {
	if p == nil || len(p.searchHighlights) == 0 {
		return nil, nil
	}
	spans, ok := p.searchHighlights[lineIdx]
	if !ok || len(spans) == 0 {
		return nil, nil
	}
	sortSpansByStart(spans)
	adjusted := shiftAndClipSpans(spans, drop, widthLimit)

	var focusSpans []textSpan
	if hit := p.focusedHit(); hit != nil {
		if !p.binaryMode {
			if hit.line == lineIdx {
				if sp, ok := adjustSpan(hit.span, drop, widthLimit); ok {
					focusSpans = append(focusSpans, sp)
				}
			}
		} else if hit.len > 0 || hit.nibblePos >= 0 {
			bytesPerLine := binaryPreviewLineWidth
			if p.binarySource != nil && p.binarySource.bytesPerLine > 0 {
				bytesPerLine = p.binarySource.bytesPerLine
			}
			start := hit.startByte
			if start < 0 {
				start = hit.line*bytesPerLine + byteIndexForSpanStart(hit.span.start, bytesPerLine)
			}
			end := start + hit.len - 1
			nibble := hit.nibblePos
			startLine := start / bytesPerLine
			endLine := end / bytesPerLine
			if nibble >= 0 {
				if nl := nibble / bytesPerLine; nl > endLine {
					endLine = nl
				}
			}
			if lineIdx >= startLine && lineIdx <= endLine {
				lineStart := lineIdx * bytesPerLine
				lineEnd := lineStart + bytesPerLine - 1
				if hit.len > 0 {
					for b := maxInt(lineStart, start); b <= minInt(lineEnd, end); b++ {
						col := b - lineStart
						if sp, ok := adjustSpan(hexSpanForByte(col, bytesPerLine), drop, widthLimit); ok {
							focusSpans = append(focusSpans, sp)
						}
						if sp, ok := adjustSpan(asciiSpanForByte(col, bytesPerLine), drop, widthLimit); ok {
							focusSpans = append(focusSpans, sp)
						}
					}
				}
				if nibble >= 0 && nibble >= lineStart && nibble <= lineEnd {
					col := nibble - lineStart
					if sp, ok := adjustSpan(hexNibbleSpanForByte(col, bytesPerLine, true), drop, widthLimit); ok {
						focusSpans = append(focusSpans, sp)
					}
					if sp, ok := adjustSpan(asciiSpanForByte(col, bytesPerLine), drop, widthLimit); ok {
						focusSpans = append(focusSpans, sp)
					}
				}
			}
		}
	}
	return adjusted, focusSpans
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

func hexNibbleSpanForByte(byteIdx int, bytesPerLine int, high bool) textSpan {
	span := hexSpanForByte(byteIdx, bytesPerLine)
	if high {
		span.end = span.start + 1
	} else {
		span.start++
	}
	if span.end <= span.start {
		span.end = span.start + 1
	}
	return span
}

func asciiSpanForByte(byteIdx int, bytesPerLine int) textSpan {
	if byteIdx < 0 {
		byteIdx = 0
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	if byteIdx >= bytesPerLine {
		byteIdx = bytesPerLine - 1
	}
	asciiStart := 10 + bytesPerLine*3 + 3
	return textSpan{start: asciiStart + byteIdx, end: asciiStart + byteIdx + 1}
}

func parseBinaryNeedle(query string) ([]byte, bool, byte, error) {
	if query == "" {
		return nil, false, 0, nil
	}
	if strings.HasPrefix(query, ":") {
		hex := strings.TrimSpace(query[1:])
		hex = strings.ReplaceAll(hex, " ", "")
		if len(hex) == 0 {
			return nil, false, 0, nil
		}
		buf := make([]byte, 0, len(hex)/2)
		var partial bool
		var nibble byte
		for i := 0; i < len(hex); i += 2 {
			hi, ok := hexNibble(hex[i])
			if !ok {
				return nil, false, 0, errors.New("invalid hex pattern")
			}
			if i+1 >= len(hex) {
				partial = true
				nibble = hi
				break
			}
			lo, ok := hexNibble(hex[i+1])
			if !ok {
				return nil, false, 0, errors.New("invalid hex pattern")
			}
			buf = append(buf, hi<<4|lo)
		}
		return buf, partial, nibble, nil
	}
	return []byte(query), false, 0, nil
}

func hexNibble(ch byte) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	default:
		return 0, false
	}
}

func findBinaryPattern(buf []byte, needle []byte, partialNibble bool, nibble byte, start int) int {
	patternLen := len(needle)
	if partialNibble {
		patternLen++
	}
	if patternLen == 0 || start >= len(buf) {
		return -1
	}
	if start < 0 {
		start = 0
	}
	if !partialNibble {
		idx := bytes.Index(buf[start:], needle)
		if idx == -1 {
			return -1
		}
		return start + idx
	}

	if patternLen == 1 {
		for i := start; i < len(buf); i++ {
			if buf[i]>>4 == nibble {
				return i
			}
		}
		return -1
	}

	searchFrom := start
	limit := len(buf) - patternLen
	for searchFrom <= limit {
		idx := bytes.Index(buf[searchFrom:], needle)
		if idx == -1 {
			return -1
		}
		candidate := searchFrom + idx
		nextIdx := candidate + len(needle)
		if nextIdx < len(buf) && buf[nextIdx]>>4 == nibble {
			return candidate
		}
		searchFrom = candidate + 1
	}
	return -1
}

func smartCaseInsensitiveASCII(b []byte) bool {
	for _, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			return false
		}
	}
	return true
}

func foldASCIIBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			out[i] = ch + 32
		} else {
			out[i] = ch
		}
	}
	return out
}

func applySearchHighlights(text string, spans []textSpan, focus []textSpan) string {
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

		g := uniseg.NewGraphemes(text[i:])
		if !g.Next() {
			break
		}
		cluster := g.Str()
		clusterWidth := textutil.DisplayWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}

		if !active && col >= current.start {
			currentStyle = highlightStyleForSpan(current, focus)
			builder.WriteString(currentStyle)
			active = true
		}

		builder.WriteString(cluster)
		col += clusterWidth

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

		i += len(cluster)
	}

	if active {
		builder.WriteString(highlightOffForStyle(currentStyle))
		if activeSGR != "" {
			builder.WriteString(activeSGR)
		}
	}
	return builder.String()
}

func highlightStyleForSpan(span textSpan, focuses []textSpan) string {
	for _, f := range focuses {
		if spansOverlap(span, f) {
			return searchHighlightFocusOn
		}
	}
	return searchHighlightOn
}

func highlightOffForStyle(style string) string {
	if style == searchHighlightFocusOn {
		return searchHighlightFocusOff
	}
	return searchHighlightOff
}

func spansOverlap(a, b textSpan) bool {
	return a.start < b.end && b.start < a.end
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func byteIndexForSpanStart(startCol int, bytesPerLine int) int {
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	for b := 0; b < bytesPerLine; b++ {
		if hexSpanForByte(b, bytesPerLine).start == startCol {
			return b
		}
	}
	return -1
}

func sortSpansByStart(spans []textSpan) {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end < spans[j].end
		}
		return spans[i].start < spans[j].start
	})
}

func visualizeSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "·")
}

func (p *PreviewPager) enterTextSearchMode() {
	p.enterSearchModeWithPreset(false, nil)
}

func (p *PreviewPager) enterBinarySearchMode() {
	if p == nil || !p.binaryMode {
		return
	}
	p.enterSearchModeWithPreset(true, []rune{':'})
}

func (p *PreviewPager) enterSearchModeWithPreset(binary bool, preset []rune) {
	if p == nil {
		return
	}
	// Only allow binary search mode when in binary preview; otherwise treat as text.
	binary = binary && p.binaryMode
	p.searchMode = true
	p.searchBinaryMode = binary
	p.searchFullScan = false
	if len(preset) > 0 && (len(p.searchQuery) == 0 || p.searchQueryBinary != binary) {
		p.searchInput = append([]rune(nil), preset...)
	} else if len(p.searchQuery) > 0 && p.searchQueryBinary == binary {
		p.searchInput = append([]rune(nil), []rune(p.searchQuery)...)
	} else {
		p.searchInput = nil
	}
	p.searchErr = nil
}

func (p *PreviewPager) exitSearchMode() {
	p.searchMode = false
	p.searchBinaryMode = false
	p.searchFullScan = false
	p.searchInput = nil
	p.stopSearchTimer()
}

func (p *PreviewPager) cancelSearch() {
	p.searchMode = false
	p.searchBinaryMode = false
	p.searchFullScan = false
	p.searchInput = nil
	p.stopSearchTimer()
	p.searchQuery = ""
	p.searchQueryBinary = false
	p.clearSearchResults()
}

func (p *PreviewPager) toggleSearchBinaryMode() {
	if p == nil || !p.searchMode {
		return
	}
	if !p.binaryMode {
		p.searchBinaryMode = false
		return
	}
	p.searchBinaryMode = !p.searchBinaryMode
	if p.searchBinaryMode {
		if len(p.searchInput) == 0 {
			p.searchInput = []rune{':'}
		} else if p.searchInput[0] != ':' {
			p.searchInput = append([]rune{':'}, p.searchInput...)
		}
	} else if len(p.searchInput) > 0 && p.searchInput[0] == ':' {
		p.searchInput = p.searchInput[1:]
	}
	p.onSearchInputChanged()
}

func (p *PreviewPager) toggleSearchLimit() {
	if p == nil || !p.searchMode || !p.binaryMode {
		return
	}
	p.searchFullScan = !p.searchFullScan
	p.onSearchInputChanged()
}

func (p *PreviewPager) appendSearchRune(ch rune) {
	if ch == 0 {
		return
	}
	if p.searchMode && p.searchBinaryMode && (len(p.searchInput) == 0 || (len(p.searchInput) > 0 && p.searchInput[0] != ':')) {
		p.searchInput = append([]rune{':'}, p.searchInput...)
	}
	p.searchInput = append(p.searchInput, ch)
	p.drainSearchBuffer()
	p.onSearchInputChanged()
}

// drainSearchBuffer pulls buffered runes (e.g., from a paste burst) into the search input
// without blocking; stops if an escape sequence prefix is encountered.
func (p *PreviewPager) drainSearchBuffer() {
	if p == nil || !p.searchMode || p.reader == nil {
		return
	}
	for p.reader.Buffered() > 0 {
		r, _, err := p.reader.ReadRune()
		if err != nil {
			return
		}
		if r < 32 || r == '\x1b' {
			_ = p.reader.UnreadRune()
			return
		}
		p.searchInput = append(p.searchInput, r)
	}
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
	searchBinaryUI := p.searchBinaryMode
	if !p.searchMode && p.searchQueryBinary {
		searchBinaryUI = true
	}
	p.searchQueryBinary = searchBinaryUI

	binaryEngine := p.binaryMode
	if query == "" {
		return
	}

	var (
		hits       []searchHit
		highlights map[int][]textSpan
		limited    bool
		err        error
	)

	if binaryEngine {
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

	needle, partialNibble, nibble, err := parseBinaryNeedle(query)
	if err != nil {
		return nil, nil, false, err
	}
	if len(needle) == 0 && !partialNibble {
		return nil, nil, false, nil
	}

	hexQuery := strings.HasPrefix(query, ":")

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

	bytesToScan := totalBytes
	if !p.searchFullScan && searchMaxBinaryBytes > 0 && bytesToScan > searchMaxBinaryBytes {
		bytesToScan = searchMaxBinaryBytes
	}

	bufSize := p.binarySource.chunkSize
	if bufSize < bytesPerLine {
		bufSize = bytesPerLine * 64
	}
	patternLen := len(needle)
	if partialNibble {
		patternLen++
	}
	overlap := patternLen - 1
	if overlap < 0 {
		overlap = 0
	}
	window := make([]byte, bufSize+overlap)
	caseInsensitive := !hexQuery && smartCaseInsensitiveASCII(needle)
	needleFolded := needle
	if caseInsensitive {
		needleFolded = foldASCIIBytes(needle)
	}

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
		searchChunk := chunk
		if caseInsensitive {
			searchChunk = foldASCIIBytes(chunk)
		}

		searchFrom := 0
		for len(hits) < searchMaxHits {
			idx := findBinaryPattern(searchChunk, needleFolded, partialNibble, nibble, searchFrom)
			if idx == -1 {
				break
			}
			abs := offset - int64(len(tail)) + int64(idx)
			startByte := int(abs)
			matchLen := len(needle)
			endByte := startByte + matchLen - 1
			nibbleByte := -1
			if partialNibble {
				nibbleByte = startByte + matchLen
			}
			startLine := startByte / bytesPerLine
			endLine := endByte / bytesPerLine
			if nibbleByte >= 0 {
				if nl := nibbleByte / bytesPerLine; nl > endLine {
					endLine = nl
				}
			}

			for line := startLine; line <= endLine; line++ {
				lineStart := line * bytesPerLine
				lineEnd := lineStart + bytesPerLine - 1
				for b := maxInt(lineStart, startByte); b <= minInt(lineEnd, endByte); b++ {
					col := b - lineStart
					highlights[line] = append(highlights[line],
						hexSpanForByte(col, bytesPerLine),
						asciiSpanForByte(col, bytesPerLine),
					)
				}
				if nibbleByte >= 0 && nibbleByte >= lineStart && nibbleByte <= lineEnd {
					col := nibbleByte - lineStart
					highlights[line] = append(highlights[line],
						hexNibbleSpanForByte(col, bytesPerLine, true),
						asciiSpanForByte(col, bytesPerLine),
					)
				}
			}

			hits = append(hits, searchHit{
				line:      startLine,
				span:      hexSpanForByte(startByte-startLine*bytesPerLine, bytesPerLine),
				len:       matchLen,
				startByte: startByte,
				nibbleEnd: partialNibble,
				nibblePos: nibbleByte,
			})

			step := len(needle)
			if step < 1 {
				step = 1
			}
			if partialNibble {
				step = len(needle) + 1 // skip over nibble tail to avoid overlapping partial matches
			}
			searchFrom = idx + step
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
