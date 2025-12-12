package render

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

func (r *Renderer) cachedRuneWidth(ru rune) int {
	return textutil.DisplayWidth(string(ru))
}

func (r *Renderer) measureTextWidth(text string) int {
	return textutil.DisplayWidth(text)
}

func (r *Renderer) truncateTextToWidth(text string, maxWidth int) string {
	if maxWidth <= 0 || text == "" {
		return ""
	}

	if r.measureTextWidth(text) <= maxWidth {
		return text
	}

	const ellipsis = "…"
	ellipsisWidth := textutil.DisplayWidth(ellipsis)
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	if maxWidth <= ellipsisWidth {
		return ellipsis
	}

	available := maxWidth - ellipsisWidth
	var builder strings.Builder
	currentWidth := 0

	g := uniseg.NewGraphemes(text)
	for g.Next() {
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w <= 0 {
			w = 1
		}
		if currentWidth+w > available {
			break
		}
		builder.WriteString(cluster)
		currentWidth += w
	}

	builder.WriteString(ellipsis)
	return builder.String()
}

func (r *Renderer) drawTextLine(startX, y, maxWidth int, text string, style tcell.Style) int {
	x := startX
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		if x-startX >= maxWidth {
			break
		}
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w <= 0 {
			w = 1
		}
		// Do not manually paint padding for wide clusters here.
		// tcell tracks continuation cells for double-width graphemes; overwriting
		// them (e.g., with spaces) can leave ghost characters in Windows Terminal.
		runes := []rune(cluster)
		main := runes[0]
		comb := runes[1:]
		r.screen.SetContent(x, y, main, comb, style)
		x += w
	}
	return x
}

func (r *Renderer) drawStyledRune(x, y, maxX int, ru rune, style tcell.Style) int {
	if x >= maxX {
		return x
	}

	width := textutil.DisplayWidth(string(ru))
	if width <= 0 {
		width = 1
	}

	r.screen.SetContent(x, y, ru, nil, style)
	return x + width
}

func (r *Renderer) clipTextToWidth(text string, maxWidth int) (string, bool) {
	if maxWidth <= 0 {
		return "", text != ""
	}

	var builder strings.Builder
	width := 0
	truncated := false
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		cluster := g.Str()
		rw := textutil.DisplayWidth(cluster)
		if rw <= 0 {
			rw = 1
		}
		if width+rw > maxWidth {
			truncated = true
			break
		}
		builder.WriteString(cluster)
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
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		if x >= maxX {
			break
		}
		cluster := g.Str()
		w := textutil.DisplayWidth(cluster)
		if w <= 0 {
			w = 1
		}
		runes := []rune(cluster)
		main := runes[0]
		comb := runes[1:]
		r.screen.SetContent(x, y, main, comb, style)
		x += w
	}
	return x
}

func (r *Renderer) drawHighlightedText(startX, y, maxX int, text string, spans []highlightSpan, offset int, baseStyle, highlightStyle tcell.Style) (int, int) {
	if maxX <= startX {
		return startX, offset + len([]rune(text))
	}

	x := startX
	spanIdx := 0
	runeOffset := offset

	g := uniseg.NewGraphemes(text)
	for g.Next() {
		if x >= maxX {
			break
		}
		cluster := g.Str()
		clusterRunes := []rune(cluster)
		clusterRuneCount := len(clusterRunes)
		clusterWidth := textutil.DisplayWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}

		for spanIdx < len(spans) && runeOffset >= spans[spanIdx].end {
			spanIdx++
		}
		style := baseStyle
		if spanIdx < len(spans) && runeOffset >= spans[spanIdx].start && runeOffset < spans[spanIdx].end {
			style = highlightStyle
		}

		main := clusterRunes[0]
		comb := clusterRunes[1:]
		r.screen.SetContent(x, y, main, comb, style)
		x += clusterWidth
		runeOffset += clusterRuneCount
	}

	return x, runeOffset
}
