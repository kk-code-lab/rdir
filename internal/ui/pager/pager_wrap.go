package pager

import (
	"sort"
	"strings"
	"unicode/utf8"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	"github.com/rivo/uniseg"
)

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
