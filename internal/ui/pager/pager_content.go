package pager

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

type pagerContentKind int

const (
	pagerContentUnknown pagerContentKind = iota
	pagerContentText
	pagerContentMarkdown
	pagerContentJSON
	pagerContentBinary
)

func (p *PreviewPager) contentKind() pagerContentKind {
	if p == nil || p.state == nil || p.state.PreviewData == nil {
		return pagerContentUnknown
	}
	preview := p.state.PreviewData
	switch {
	case len(preview.BinaryInfo.Lines) > 0:
		return pagerContentBinary
	case preview.FormattedKind == "markdown":
		return pagerContentMarkdown
	case len(preview.FormattedTextLines) > 0:
		name := strings.ToLower(filepath.Ext(preview.Name))
		if name == ".json" {
			return pagerContentJSON
		}
		return pagerContentText
	case len(preview.TextLines) > 0 || preview.LineCount > 0:
		return pagerContentText
	default:
		return pagerContentUnknown
	}
}

func contentKindLabel(kind pagerContentKind) string {
	switch kind {
	case pagerContentBinary:
		return "binary"
	case pagerContentMarkdown:
		return "markdown"
	case pagerContentJSON:
		return "json"
	case pagerContentText:
		return "text"
	default:
		return "file"
	}
}

func (p *PreviewPager) prepareContent() {
	lines, charCount, binarySource, textSource := p.buildContentLines()
	if binarySource != nil {
		p.binaryMode = true
		p.wrapEnabled = false
		p.binarySource = binarySource
		p.rawTextSource = nil
		p.lines = nil
		p.lineWidths = nil
		p.charCount = charCount
		return
	}
	p.binaryMode = false
	p.binarySource = nil
	p.rawTextSource = textSource

	if textSource != nil {
		p.lines = nil
		p.lineWidths = nil
		p.rawLines = nil
		p.rawLineWidths = nil
		p.rawSanitized = nil
		p.rawSanitizedWid = nil
		if p.state != nil && p.state.PreviewScrollOffset > 0 {
			// Preload up to the remembered scroll position so reopening the pager
			// lands where the user left off, even when the file was previously only
			// partially streamed.
			_ = textSource.EnsureLine(p.state.PreviewScrollOffset)
		}
		p.charCount = textSource.CharCount()
	} else {
		if len(lines) == 0 {
			lines = []string{""}
		}
		widths := make([]int, len(lines))
		sanitized := make([]string, len(lines))
		sanitizedWidths := make([]int, len(lines))
		for i, line := range lines {
			widths[i] = displayWidth(line)
			safe := textutil.SanitizeTerminalText(line)
			sanitized[i] = safe
			sanitizedWidths[i] = displayWidth(safe)
		}
		p.lines = lines
		p.lineWidths = widths
		p.rawLines = lines
		p.rawLineWidths = widths
		p.rawSanitized = sanitized
		p.rawSanitizedWid = sanitizedWidths
		p.charCount = charCount
	}

	if preview := p.state.PreviewData; preview != nil {
		if len(preview.FormattedSegments) > 0 {
			formatted := make([]string, len(preview.FormattedSegments))
			widths := make([]int, len(preview.FormattedSegments))
			rules := make([]bool, len(preview.FormattedSegments))
			ruleStyles := make([]string, len(preview.FormattedSegments))
			for i, line := range preview.FormattedSegments {
				formatted[i], rules[i], ruleStyles[i] = ansiFromSegments(line)
				if i < len(preview.FormattedSegmentLineMeta) && preview.FormattedSegmentLineMeta[i].DisplayWidth > 0 {
					widths[i] = preview.FormattedSegmentLineMeta[i].DisplayWidth
				} else {
					widths[i] = segmentDisplayWidth(line)
				}
			}
			p.formattedLines = formatted
			p.formattedWidths = widths
			p.formattedRules = rules
			p.formattedStyles = ruleStyles
		} else if len(preview.FormattedTextLines) > 0 {
			p.formattedLines = append([]string(nil), preview.FormattedTextLines...)
			p.formattedWidths = make([]int, len(p.formattedLines))
			for i, line := range p.formattedLines {
				p.formattedWidths[i] = displayWidth(line)
			}
			p.formattedRules = nil
			p.formattedStyles = nil
		} else {
			p.formattedLines = nil
			p.formattedWidths = nil
			p.formattedRules = nil
			p.formattedStyles = nil
		}
	}

	p.applyFormatPreference(true)
}

func (p *PreviewPager) applyFormatPreference(initial bool) {
	preferRaw := p.state != nil && p.state.PreviewPreferRaw
	if len(p.formattedLines) == 0 {
		p.showFormatted = false
	} else {
		p.showFormatted = !preferRaw
	}
	p.updateDisplayLines()
}

func (p *PreviewPager) updateDisplayLines() {
	if p.showFormatted {
		p.lines = p.formattedLines
		p.lineWidths = p.formattedWidths
	} else {
		if len(p.rawSanitized) > 0 {
			p.lines = p.rawSanitized
			p.lineWidths = p.rawSanitizedWid
		} else {
			p.lines = p.rawLines
			p.lineWidths = p.rawLineWidths
		}
	}
}

func (p *PreviewPager) toggleFormatView() {
	if len(p.formattedLines) == 0 {
		return
	}
	p.showFormatted = !p.showFormatted
	if p.state != nil {
		p.state.PreviewPreferRaw = !p.showFormatted
		p.state.PreviewScrollOffset = 0
		p.state.PreviewWrapOffset = 0
	}
	p.updateDisplayLines()
	p.rowSpans = nil
	p.rowPrefix = nil
	p.resetWrapCache()
	if p.searchQuery != "" {
		p.executeSearch(p.searchQuery)
	}
}

func (p *PreviewPager) buildContentLines() ([]string, int, *binaryPagerSource, *textPagerSource) {
	if p.state == nil || p.state.PreviewData == nil {
		return nil, 0, nil, nil
	}

	preview := p.state.PreviewData
	switch {
	case preview.IsDir:
		lines := formatDirectoryPreview(preview)
		return lines, lineCharCount(lines), nil, nil
	case len(preview.TextLines) > 0:
		if preview.TextTruncated && len(preview.TextLineMeta) == len(preview.TextLines) {
			filePath := filepath.Join(p.state.CurrentPath, preview.Name)
			if source, err := newTextPagerSource(filePath, preview); err == nil {
				return nil, preview.TextCharCount, nil, source
			}
		}
		return preview.TextLines, preview.TextCharCount, nil, nil
	case len(preview.BinaryInfo.Lines) > 0:
		filePath := filepath.Join(p.state.CurrentPath, preview.Name)
		source, err := newBinaryPagerSource(filePath, preview.BinaryInfo.TotalBytes, p.width)
		if err == nil {
			return nil, int(preview.BinaryInfo.TotalBytes), source, nil
		}
		lines := append([]string(nil), preview.BinaryInfo.Lines...)
		if len(lines) > 0 {
			lines = lines[1:]
		}
		return lines, lineCharCount(lines), nil, nil
	default:
		lines := []string{"(no preview available)"}
		return lines, lineCharCount(lines), nil, nil
	}
}

func lineCharCount(lines []string) int {
	total := 0
	for _, line := range lines {
		total += utf8.RuneCountInString(line)
	}
	return total
}
