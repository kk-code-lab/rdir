package render

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

func (r *Renderer) drawPreviewPanel(state *statepkg.AppState, layout layoutMetrics, w, h int) {
	startX := layout.previewStart
	panelWidth := layout.previewWidth
	if panelWidth <= 0 {
		return
	}

	baseStyle := tcell.StyleDefault.Background(r.theme.PreviewBg).Foreground(r.theme.PreviewFg)

	for y := 1; y < h; y++ {
		for x := startX; x < startX+panelWidth && x < w; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseStyle)
		}
	}

	y := 1
	if state.PreviewData == nil {
		placeholder := " preview unavailable "
		r.drawTextLine(startX, y, panelWidth, placeholder, baseStyle.Dim(true))
		y++
		for y < h-1 {
			for x := startX; x < startX+panelWidth && x < w; x++ {
				r.screen.SetContent(x, y, ' ', nil, baseStyle)
			}
			y++
		}
		return
	}

	var preview *statepkg.PreviewData
	wrapEnabled := false
	if state != nil {
		preview = state.PreviewData
		wrapEnabled = state.PreviewFullScreen && state.PreviewWrap
	}
	bottomLimit := h - 1
	startIdx := state.PreviewScrollOffset
	if startIdx < 0 {
		startIdx = 0
	}

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
		if startIdx > len(preview.DirEntries) {
			startIdx = len(preview.DirEntries)
		}
		for i := startIdx; i < len(preview.DirEntries); i++ {
			entry := preview.DirEntries[i]
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
			displayName := textutil.SanitizeTerminalText(entry.Name)
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
	} else if !preview.IsDir && (len(preview.TextLines) > 0 || len(preview.FormattedTextLines) > 0) {
		lines, meta := previewTextLines(preview)
		textStyle := baseStyle.Foreground(r.theme.FileFg)
		if startIdx > len(lines) {
			startIdx = len(lines)
		}
		for i := startIdx; i < len(lines); i++ {
			line := lines[i]
			safeLine := textutil.SanitizeTerminalText(line)
			lineWidth := r.previewLineWidth(meta, i, safeLine)
			if wrapEnabled {
				if !r.drawWrappedPreviewText(safeLine, startX, panelWidth, textStyle, &y, bottomLimit, w) {
					break
				}
			} else {
				if !r.drawPreviewTextLineClipped(safeLine, lineWidth, startX, panelWidth, textStyle, y, bottomLimit, w) {
					break
				}
				y++
			}
		}
	} else if !preview.IsDir && len(preview.BinaryInfo.Lines) > 0 {
		textStyle := baseStyle.Foreground(r.theme.FileFg)
		if startIdx > len(preview.BinaryInfo.Lines) {
			startIdx = len(preview.BinaryInfo.Lines)
		}
		for i := startIdx; i < len(preview.BinaryInfo.Lines); i++ {
			line := preview.BinaryInfo.Lines[i]
			line = textutil.SanitizeTerminalText(line)
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

func (r *Renderer) drawPreviewTextLineClipped(text string, lineWidth int, startX, panelWidth int, style tcell.Style, y, bottomLimit, screenWidth int) bool {
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
		effectiveWidth := lineWidth
		if effectiveWidth <= 0 {
			effectiveWidth = r.measureTextWidth(text)
		}
		if effectiveWidth > displayWidth {
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
		effectiveWidth := lineWidth
		if effectiveWidth <= 0 {
			effectiveWidth = r.measureTextWidth(text)
		}
		truncated = effectiveWidth > 1
	}

	if truncated && available > 0 {
		indicatorX := startX + available - 1
		if indicatorX < screenWidth && indicatorX >= startX {
			r.screen.SetContent(indicatorX, y, '…', nil, style)
		}
	}

	return true
}

func (r *Renderer) drawWrappedPreviewText(text string, startX, panelWidth int, style tcell.Style, y *int, bottomLimit, screenWidth int) bool {
	if panelWidth <= 0 || y == nil {
		return false
	}
	segments := r.wrapPreviewText(text, panelWidth)
	if len(segments) == 0 {
		segments = []string{""}
	}
	for _, segment := range segments {
		if *y >= bottomLimit {
			return false
		}
		_ = r.drawPreviewTextLineClipped(segment, 0, startX, panelWidth, style, *y, bottomLimit, screenWidth)
		*y++
	}
	return true
}

func (r *Renderer) wrapPreviewText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return nil
	}
	var segments []string
	var builder strings.Builder
	currentWidth := 0

	flush := func() {
		segments = append(segments, builder.String())
		builder.Reset()
		currentWidth = 0
	}

	for _, ru := range text {
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth <= 0 {
			runeWidth = 1
		}
		if currentWidth > 0 && currentWidth+runeWidth > maxWidth {
			flush()
		}
		if runeWidth > maxWidth {
			segments = append(segments, string(ru))
			currentWidth = 0
			builder.Reset()
			continue
		}
		builder.WriteRune(ru)
		currentWidth += runeWidth
	}

	if builder.Len() > 0 {
		flush()
	}

	if len(segments) == 0 {
		segments = append(segments, "")
	}

	return segments
}

func (r *Renderer) drawFullScreenPreview(state *statepkg.AppState, w, h int) {
	layout := layoutMetrics{
		previewStart: 0,
		previewWidth: w,
		showPreview:  true,
		binaryMode:   r.fullScreenBinaryMode(state, w),
	}
	r.drawPreviewPanel(state, layout, w, h)
}

func (r *Renderer) fullScreenBinaryMode(state *statepkg.AppState, width int) binaryPreviewMode {
	if !r.previewContainsBinary(state) {
		return binaryPreviewModeNone
	}
	switch {
	case width >= binaryFullPreviewMinWidth:
		return binaryPreviewModeFull
	case width >= binaryHexPreviewMinWidth:
		return binaryPreviewModeHexOnly
	default:
		return binaryPreviewModeNone
	}
}

func previewTextLines(preview *statepkg.PreviewData) ([]string, []statepkg.TextLineMetadata) {
	if preview == nil {
		return nil, nil
	}
	if len(preview.FormattedTextLines) > 0 {
		return preview.FormattedTextLines, preview.FormattedTextLineMeta
	}
	return preview.TextLines, preview.TextLineMeta
}

func (r *Renderer) previewLineWidth(meta []statepkg.TextLineMetadata, idx int, text string) int {
	if len(meta) > 0 && idx >= 0 && idx < len(meta) {
		if width := meta[idx].DisplayWidth; width > 0 {
			return width
		}
	}
	return r.measureTextWidth(text)
}
