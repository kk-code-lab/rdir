package render

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func (r *Renderer) drawPreviewPanel(state *statepkg.AppState, layout layoutMetrics, w, h int) {
	startX := layout.previewStart
	panelWidth := layout.previewWidth
	if panelWidth <= 0 {
		return
	}

	baseStyle := tcell.StyleDefault.Background(r.theme.SidebarBg).Foreground(r.theme.SidebarFg)

	for y := 1; y < h; y++ {
		for x := startX; x < startX+panelWidth && x < w; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseStyle)
		}
	}

	y := 1
	if state.PreviewData == nil {
		for y < h-1 {
			for x := startX; x < startX+panelWidth && x < w; x++ {
				r.screen.SetContent(x, y, ' ', nil, baseStyle)
			}
			y++
		}
		return
	}

	preview := state.PreviewData
	bottomLimit := h - 1

	drawLine := func(text string, style tcell.Style) bool {
		if y >= bottomLimit {
			return false
		}
		endX := r.drawTextLine(startX, y, panelWidth, text, style)
		for x := endX; x < startX+panelWidth && x < w; x++ {
			r.screen.SetContent(x, y, ' ', nil, style)
		}
		y++
		return true
	}

	if preview.IsDir && len(preview.DirEntries) > 0 {
		for _, entry := range preview.DirEntries {
			var rowStyle tcell.Style
			if entry.IsSymlink {
				rowStyle = baseStyle.Foreground(r.theme.SymlinkFg)
			} else if entry.IsDir {
				rowStyle = baseStyle.Foreground(r.theme.DirectoryFg)
			} else {
				rowStyle = baseStyle.Foreground(r.theme.FileFg)
			}
			icon := " "
			if entry.IsSymlink {
				icon = "@"
			} else if entry.IsDir {
				icon = "/"
			}
			prefix := fmt.Sprintf(" %s ", icon)
			nameWidth := panelWidth - r.measureTextWidth(prefix)
			displayName := entry.Name
			if nameWidth > 0 {
				displayName = r.truncateTextToWidth(displayName, nameWidth)
			} else {
				displayName = ""
			}
			text := prefix + displayName
			if !drawLine(text, rowStyle) {
				break
			}
		}
	} else if !preview.IsDir && len(preview.TextLines) > 0 {
		textStyle := baseStyle.Foreground(r.theme.FileFg)
		for _, line := range preview.TextLines {
			expanded := r.expandTabs(line, previewTabWidth)
			if !r.drawPreviewTextLineClipped(expanded, startX, panelWidth, textStyle, y, bottomLimit, w) {
				break
			}
			y++
		}
	} else if !preview.IsDir && len(preview.BinaryInfo.Lines) > 0 {
		textStyle := baseStyle.Foreground(r.theme.FileFg)
		for _, line := range preview.BinaryInfo.Lines {
			if !strings.Contains(line, "|") {
				if !drawLine(line, textStyle) {
					break
				}
				continue
			}
			if layout.binaryMode == binaryPreviewModeNone {
				if !drawLine(line, textStyle) {
					break
				}
				continue
			}
			if !r.drawBinaryPreviewLine(line, startX, panelWidth, layout.binaryMode, textStyle, y, bottomLimit, w) {
				break
			}
			y++
		}
	}

	for y < bottomLimit {
		for x := startX; x < startX+panelWidth && x < w; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseStyle)
		}
		y++
	}
}

func (r *Renderer) drawBinaryPreviewLine(line string, startX, panelWidth int, mode binaryPreviewMode, style tcell.Style, y, bottomLimit, screenWidth int) bool {
	if panelWidth <= 0 || y >= bottomLimit || mode == binaryPreviewModeNone {
		return false
	}

	limit := startX + panelWidth
	if limit > screenWidth {
		limit = screenWidth
	}

	for x := startX; x < limit; x++ {
		r.screen.SetContent(x, y, ' ', nil, style)
	}

	renderText := line
	if mode == binaryPreviewModeHexOnly {
		if idx := strings.Index(renderText, " |"); idx >= 0 {
			renderText = strings.TrimRight(renderText[:idx], " ")
		}
	}

	padding := previewInnerPadding
	if panelWidth <= padding*2 {
		padding = 0
	}

	drawStart := startX + padding
	maxWidth := panelWidth - padding*2
	if maxWidth <= 0 {
		return true
	}

	r.drawTextLine(drawStart, y, maxWidth, renderText, style)
	return true
}

func (r *Renderer) drawPreviewTextLineClipped(text string, startX, panelWidth int, style tcell.Style, y, bottomLimit, screenWidth int) bool {
	if panelWidth <= 0 || y >= bottomLimit {
		return false
	}

	available := panelWidth
	if screenWidth-startX < available {
		available = screenWidth - startX
		if available <= 0 {
			return false
		}
	}

	for x := startX; x < startX+available && x < screenWidth; x++ {
		r.screen.SetContent(x, y, ' ', nil, style)
	}

	renderWidth := available
	truncated := false
	if renderWidth > 1 {
		displayWidth := renderWidth
		if r.measureTextWidth(text) > displayWidth {
			displayWidth--
			clipped, wasTruncated := r.clipTextToWidth(text, displayWidth)
			truncated = wasTruncated
			if displayWidth > 0 {
				r.drawTextLine(startX, y, displayWidth, clipped, style)
			}
		} else {
			r.drawTextLine(startX, y, displayWidth, text, style)
		}
	} else if renderWidth == 1 {
		truncated = r.measureTextWidth(text) > 1
	}

	if truncated && available > 0 {
		indicatorX := startX + available - 1
		if indicatorX < screenWidth && indicatorX >= startX {
			r.screen.SetContent(indicatorX, y, '…', nil, style)
		}
	}

	return true
}
