package render

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

type helpOverlayEntry struct {
	keys string
	desc string
}

type helpOverlaySection struct {
	title   string
	entries []helpOverlayEntry
}

func buildHelpOverlayLines(state *statepkg.AppState) []string {
	hiddenDesc := "Hide hidden files"
	if state != nil && state.HideHiddenFiles {
		hiddenDesc = "Show hidden files"
	}

	sections := []helpOverlaySection{
		{
			title: "Navigation",
			entries: []helpOverlayEntry{
				{keys: "↑/↓", desc: "Move selection"},
				{keys: "↵ or →", desc: "Open / preview selection"},
				{keys: "[ / ]", desc: "History back/forward"},
				{keys: "~", desc: "Go home"},
			},
		},
		{
			title: "Filter & Search",
			entries: []helpOverlayEntry{
				{keys: "/", desc: "Filter current directory"},
				{keys: "f", desc: "Global search"},
				{keys: "Esc", desc: "Clear or exit search/filter"},
			},
		},
		{
			title: "Preview & Pager",
			entries: []helpOverlayEntry{
				{keys: "↵ or →", desc: "Open built-in pager (fullscreen preview)"},
				{keys: "P", desc: "Open external pager ($PAGER)"},
			},
		},
		{
			title: "Actions",
			entries: []helpOverlayEntry{
				{keys: ".", desc: hiddenDesc},
				{keys: "r", desc: "Refresh directory"},
				{keys: "y", desc: "Yank path to clipboard"},
				{keys: "e", desc: "Open in external editor ($EDITOR)"},
			},
		},
		{
			title: "Exit",
			entries: []helpOverlayEntry{
				{keys: "q", desc: "Quit"},
				{keys: "x", desc: "Quit and cd here"},
				{keys: "Ctrl+C", desc: "Quit immediately"},
				{keys: "?", desc: "Close this help"},
			},
		},
	}

	lines := make([]string, 0, 32)
	for i, section := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, section.title)
		for _, entry := range section.entries {
			lines = append(lines, formatHelpOverlayEntry(entry))
		}
	}

	return lines
}

func formatHelpOverlayEntry(entry helpOverlayEntry) string {
	key := textutil.SanitizeTerminalText(entry.keys)
	desc := textutil.SanitizeTerminalText(entry.desc)
	return fmt.Sprintf("  %-14s %s", key, desc)
}

func (r *Renderer) drawHelpOverlay(state *statepkg.AppState, w, h int) {
	baseStyle := tcell.StyleDefault.Background(r.theme.Background).Foreground(r.theme.Foreground)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseStyle)
		}
	}

	title := " Help "
	headerStyle := baseStyle.Background(r.theme.FooterBg).Foreground(r.theme.FooterFg).Bold(true)
	titleStart := 0
	titleWidth := r.measureTextWidth(title)
	if w > titleWidth {
		titleStart = (w - titleWidth) / 2
	}
	r.drawTextLine(titleStart, 0, w-titleStart, title, headerStyle)

	bodyStyle := baseStyle
	lines := buildHelpOverlayLines(state)
	row := 2
	maxRow := h - 1
	for _, line := range lines {
		if row >= maxRow {
			break
		}
		text := strings.TrimRight(line, " ")
		text = r.truncateTextToWidth(text, w-4)
		r.drawTextLine(2, row, w-4, text, bodyStyle)
		row++
	}

	footer := "? toggle · Esc/q close"
	if len(footer) > 0 && h > 0 {
		footerText := r.truncateTextToWidth(footer, w)
		r.drawTextLine(0, h-1, w, footerText, headerStyle)
	}
}
