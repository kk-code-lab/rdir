package render

import statepkg "github.com/kk-code-lab/rdir/internal/state"

type binaryPreviewMode int

const (
	binaryPreviewModeNone binaryPreviewMode = iota
	binaryPreviewModeHexOnly
	binaryPreviewModeFull
)

type layoutMetrics struct {
	sidebarWidth          int
	sideSeparatorWidth    int
	contentSeparatorWidth int
	mainPanelStart        int
	mainPanelWidth        int
	previewStart          int
	previewWidth          int
	showPreview           bool
	binaryMode            binaryPreviewMode
}

const (
	minMainPanelWidth         = 32
	minPreviewPanelWidth      = 28
	minPreviewTerminalWidth   = 100
	previewListRatio          = 0.20
	previewPanelRatio         = 0.80
	previewInnerPadding       = 1
	binaryFullContentWidth    = 78
	binaryHexContentWidth     = 60
	binaryFullPreviewMinWidth = binaryFullContentWidth + (previewInnerPadding * 2)
	binaryHexPreviewMinWidth  = binaryHexContentWidth + (previewInnerPadding * 2)
	previewWidthCap           = binaryFullPreviewMinWidth
	textPreviewEstimateLines  = 40
	previewTabWidth           = 4
)

func (r *Renderer) previewContainsBinary(state *statepkg.AppState) bool {
	if state == nil || state.PreviewData == nil {
		return false
	}
	preview := state.PreviewData
	return !preview.IsDir && len(preview.BinaryInfo.Lines) > 0
}

func (r *Renderer) desiredPreviewWidth(combinedWidth, previewMin int, preview *statepkg.PreviewData) int {
	width := int(float64(combinedWidth)*previewPanelRatio + 0.5)
	if width < previewMin {
		width = previewMin
	}
	if width > previewWidthCap {
		width = previewWidthCap
	}

	if preview != nil && len(preview.TextLines) > 0 {
		needed := r.estimateTextPreviewWidth(preview) + previewInnerPadding*2
		if needed < previewMin {
			needed = previewMin
		}
		if needed > previewWidthCap {
			needed = previewWidthCap
		}
		width = needed
	}

	return width
}

func (r *Renderer) estimateTextPreviewWidth(preview *statepkg.PreviewData) int {
	if preview == nil || len(preview.TextLines) == 0 {
		return 0
	}

	limit := textPreviewEstimateLines
	if len(preview.TextLines) < limit {
		limit = len(preview.TextLines)
	}

	maxWidth := 0
	for i := 0; i < limit; i++ {
		line := r.expandTabs(preview.TextLines[i], previewTabWidth)
		width := r.measureTextWidth(line)
		if width > maxWidth {
			maxWidth = width
		}
	}

	if maxWidth > previewWidthCap {
		maxWidth = previewWidthCap
	}
	return maxWidth
}

func (r *Renderer) computeLayout(w int, state *statepkg.AppState) layoutMetrics {
	if w < 0 {
		w = 0
	}

	metrics := layoutMetrics{}
	metrics.sidebarWidth = r.sidebarWidthForWidth(w, state)
	if metrics.sidebarWidth > w {
		metrics.sidebarWidth = w
	}

	if metrics.sidebarWidth > 0 && metrics.sidebarWidth < w {
		metrics.sideSeparatorWidth = 1
	}

	metrics.mainPanelStart = metrics.sidebarWidth + metrics.sideSeparatorWidth
	contentWidth := w - metrics.mainPanelStart
	if contentWidth < 0 {
		contentWidth = 0
	}
	metrics.mainPanelWidth = contentWidth
	metrics.previewStart = w

	previewMinWidth := minPreviewPanelWidth
	isBinaryPreview := r.previewContainsBinary(state)
	if isBinaryPreview {
		previewMinWidth = binaryHexPreviewMinWidth
	}

	previewAllowed := state != nil && !state.GlobalSearchActive
	canShowPreview := previewAllowed &&
		w >= minPreviewTerminalWidth &&
		contentWidth >= (minMainPanelWidth+previewMinWidth+1)

	if canShowPreview {
		metrics.contentSeparatorWidth = 1
		combinedWidth := contentWidth - metrics.contentSeparatorWidth
		if combinedWidth >= (minMainPanelWidth + previewMinWidth) {
			previewWidth := r.desiredPreviewWidth(combinedWidth, previewMinWidth, state.PreviewData)
			mainWidth := combinedWidth - previewWidth

			if mainWidth < minMainPanelWidth {
				mainWidth = minMainPanelWidth
				previewWidth = combinedWidth - mainWidth
			}

			if previewWidth >= previewMinWidth && mainWidth >= minMainPanelWidth {
				metrics.showPreview = true
				metrics.previewWidth = previewWidth
				metrics.mainPanelWidth = mainWidth
				metrics.previewStart = metrics.mainPanelStart + metrics.mainPanelWidth + metrics.contentSeparatorWidth

				if isBinaryPreview {
					switch {
					case previewWidth >= binaryFullPreviewMinWidth:
						metrics.binaryMode = binaryPreviewModeFull
					case previewWidth >= binaryHexPreviewMinWidth:
						metrics.binaryMode = binaryPreviewModeHexOnly
					default:
						metrics.showPreview = false
					}
				}
			}
		}
	}

	if metrics.mainPanelWidth < 0 {
		metrics.mainPanelWidth = 0
	}
	if metrics.previewStart < 0 {
		metrics.previewStart = 0
	}
	if metrics.previewStart > w {
		metrics.previewStart = w
	}
	if !metrics.showPreview {
		metrics.contentSeparatorWidth = 0
		metrics.mainPanelWidth = contentWidth
		metrics.previewStart = w
		metrics.previewWidth = 0
	}

	return metrics
}

func (r *Renderer) sidebarWidthForWidth(w int, state *statepkg.AppState) int {
	if state != nil && state.GlobalSearchActive {
		return 0
	}

	switch {
	case w >= 150:
		return 28
	case w >= 120:
		return 24
	case w >= 100:
		return 20
	case w >= 80:
		return 16
	case w >= 65:
		return 12
	case w >= 52:
		return 10
	default:
		return 0
	}
}
