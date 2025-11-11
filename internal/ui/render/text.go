package render

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

func (r *Renderer) cachedRuneWidth(ru rune) int {
	if ru < 128 {
		r.runeWidthCacheMu.RLock()
		width := r.runeWidthCache[ru]
		r.runeWidthCacheMu.RUnlock()

		if width == 0 && ru != 0 {
			actualWidth := runewidth.RuneWidth(ru)
			if actualWidth < 0 {
				actualWidth = 0
			}
			r.runeWidthCacheMu.Lock()
			r.runeWidthCache[ru] = actualWidth + 1
			r.runeWidthCacheMu.Unlock()
			return actualWidth
		}
		return width - 1
	}

	if cached, ok := r.runeWidthWide.Load(ru); ok {
		return cached.(int)
	}

	width := runewidth.RuneWidth(ru)
	if width < 0 {
		width = 0
	}
	r.runeWidthWide.Store(ru, width)
	return width
}

func (r *Renderer) measureTextWidth(text string) int {
	width := 0
	for _, ru := range text {
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth < 0 {
			runeWidth = 0
		}
		width += runeWidth
	}
	return width
}

func (r *Renderer) truncateTextToWidth(text string, maxWidth int) string {
	if maxWidth <= 0 || text == "" {
		return ""
	}

	if r.measureTextWidth(text) <= maxWidth {
		return text
	}

	const ellipsis = "…"
	ellipsisWidth := r.cachedRuneWidth([]rune(ellipsis)[0])
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	if maxWidth <= ellipsisWidth {
		return ellipsis
	}

	available := maxWidth - ellipsisWidth
	var builder strings.Builder
	currentWidth := 0

	for _, ru := range text {
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth < 0 {
			runeWidth = 0
		}
		if currentWidth+runeWidth > available {
			break
		}
		builder.WriteRune(ru)
		currentWidth += runeWidth
	}

	builder.WriteString(ellipsis)
	return builder.String()
}

func (r *Renderer) expandTabs(text string, tabWidth int) string {
	if tabWidth <= 0 || !strings.ContainsRune(text, '\t') {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text) + tabWidth)
	column := 0
	for _, ru := range text {
		if ru == '\t' {
			spaces := tabWidth - (column % tabWidth)
			for i := 0; i < spaces; i++ {
				builder.WriteByte(' ')
			}
			column += spaces
			continue
		}
		builder.WriteRune(ru)
		width := r.cachedRuneWidth(ru)
		if width < 1 {
			width = 1
		}
		column += width
	}
	return builder.String()
}

func (r *Renderer) drawTextLine(startX, y, maxWidth int, text string, style tcell.Style) int {
	x := startX
	runes := []rune(text)
	i := 0

	for i < len(runes) {
		if x-startX >= maxWidth {
			break
		}

		mainc := runes[i]
		i++

		var combc []rune
		for i < len(runes) && r.cachedRuneWidth(runes[i]) < 0 {
			combc = append(combc, runes[i])
			i++
		}

		r.screen.SetContent(x, y, mainc, combc, style)

		w := r.cachedRuneWidth(mainc)
		if w < 0 {
			w = 0
		}
		x += w
	}

	return x
}

func (r *Renderer) drawStyledRune(x, y, maxX int, ru rune, style tcell.Style) int {
	if x >= maxX {
		return x
	}

	width := r.cachedRuneWidth(ru)
	if width <= 0 {
		width = 1
	}

	r.screen.SetContent(x, y, ru, nil, style)
	for w := 1; w < width && x+w < maxX; w++ {
		r.screen.SetContent(x+w, y, ' ', nil, style)
	}
	return x + width
}

func (r *Renderer) clipTextToWidth(text string, maxWidth int) (string, bool) {
	if maxWidth <= 0 {
		return "", text != ""
	}

	var builder strings.Builder
	width := 0
	truncated := false
	for _, ru := range text {
		rw := r.cachedRuneWidth(ru)
		if rw < 0 {
			rw = 0
		}
		if width+rw > maxWidth {
			truncated = true
			break
		}
		builder.WriteRune(ru)
		width += rw
	}

	if !truncated {
		return text, false
	}

	return builder.String(), true
}

func (r *Renderer) drawStyledStringClipped(startX, y, maxX int, text string, style tcell.Style) int {
	if maxX <= startX {
		return startX
	}

	x := startX
	for _, ru := range text {
		if x >= maxX {
			break
		}
		x = r.drawStyledRune(x, y, maxX, ru, style)
	}
	return x
}

func (r *Renderer) drawHighlightedText(startX, y, maxX int, text string, spans []highlightSpan, offset int, baseStyle, highlightStyle tcell.Style) (int, int) {
	runes := []rune(text)
	if maxX <= startX {
		return startX, offset + len(runes)
	}

	x := startX
	spanIdx := 0

	for idx, ru := range runes {
		if x >= maxX {
			return x, offset + len(runes)
		}

		globalIdx := offset + idx
		for spanIdx < len(spans) && globalIdx >= spans[spanIdx].end {
			spanIdx++
		}

		style := baseStyle
		if spanIdx < len(spans) && globalIdx >= spans[spanIdx].start && globalIdx < spans[spanIdx].end {
			style = highlightStyle
		}

		x = r.drawStyledRune(x, y, maxX, ru, style)
	}

	return x, offset + len(runes)
}
