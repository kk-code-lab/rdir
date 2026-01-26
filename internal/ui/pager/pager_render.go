package pager

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	renderpkg "github.com/kk-code-lab/rdir/internal/ui/render"
	"github.com/rivo/uniseg"
)

func (p *PreviewPager) render() error {
	p.updateSize()
	if p.width <= 0 {
		p.width = 1
	}
	if p.height <= 0 {
		p.height = 1
	}

	p.reflowMarkdownFormatted()

	p.ensureRowMetrics()

	if p.showHelp {
		return p.renderHelpOverlay()
	}

	header := p.headerLines()
	headerRows := len(header)
	if headerRows >= p.height {
		headerRows = p.height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	searchDisplay, cursorColBase := p.searchDisplaySegment()
	showSearchRow := false
	if searchDisplay != "" && p.searchMode {
		available := p.height - headerRows - 1 // must leave space for status
		if available >= 2 {
			showSearchRow = true
		}
	}

	contentRows := p.height - headerRows - 1 // leave space for status
	if showSearchRow {
		contentRows--
	}
	if contentRows < 1 {
		contentRows = 1
		showSearchRow = false
	}
	contentRowLimit := p.height - 1
	if showSearchRow {
		contentRowLimit--
	}
	if contentRowLimit < 1 {
		contentRowLimit = 1
	}

	if !p.showFormatted && p.rawTextSource != nil {
		target := p.state.PreviewScrollOffset + contentRows + 2
		if target < 0 {
			target = 0
		}
		p.preloadLines = target
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	p.writeString("\x1b[?25l")
	p.writeString("\x1b[2J")
	p.writeString("\x1b[H")

	row := 1
	for _, line := range header {
		if row > p.height-1 {
			break
		}
		p.drawStyledRow(row, line, false, headerBarStyle)
		row++
	}

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
		p.state.PreviewScrollOffset = start
	}
	skipRows := 0
	if p.wrapEnabled {
		skipRows = p.state.PreviewWrapOffset
	}

	for i := start; i < totalLines && row <= contentRowLimit; i++ {
		text := p.lineAt(i)
		if p.wrapEnabled && p.width > 0 {
			currentSkip := skipRows
			maxRows := contentRowLimit - row + 1
			segments := p.wrapSegmentsRangeForLine(i, text, currentSkip, maxRows)
			for segIdx, seg := range segments {
				dropCols := (currentSkip + segIdx) * p.width
				if spans, focus := p.visibleHighlights(i, dropCols, p.width); len(spans) > 0 {
					seg = applySearchHighlights(seg, spans, focus)
				}
				p.drawRow(row, seg, false)
				row++
				if row > contentRowLimit {
					break
				}
			}
			skipRows = 0
			continue
		}

		displayText := text
		if p.width > 0 {
			displayText = truncateToWidth(displayText, p.width)
		}
		if spans, focus := p.visibleHighlights(i, 0, p.width); len(spans) > 0 {
			displayText = applySearchHighlights(displayText, spans, focus)
		}
		p.drawRow(row, displayText, false)
		row++
		skipRows = 0
	}

	for row <= contentRowLimit {
		p.drawRow(row, "", false)
		row++
	}

	searchCursorRow := 0
	searchCursorCol := 0
	searchRow := contentRowLimit + 1
	if showSearchRow {
		searchDisplayPadded := " " + searchDisplay + " "
		if p.width > 0 && displayWidth(searchDisplayPadded) > p.width {
			searchDisplayPadded = truncateToWidth(searchDisplayPadded, p.width)
		}
		p.drawStyledRow(searchRow, searchDisplayPadded, true, statusBarStyle)
		searchCursorRow = searchRow
		searchCursorCol = cursorColBase + 1 // account for leading pad
		if searchCursorCol <= 0 {
			searchCursorCol = displayWidth(searchDisplayPadded)
		}
		if p.width > 0 && searchCursorCol > p.width {
			searchCursorCol = p.width
		}
	}

	status := p.statusLine(totalLines, contentRows, p.totalCharCount(), func() string {
		if showSearchRow {
			return ""
		}
		return searchDisplay
	}())
	p.drawStatus(status)
	if !showSearchRow && searchDisplay != "" && p.searchMode {
		searchCursorRow = p.height
		// search is first segment in statusLine; drawStatus wraps with leading space.
		searchCursorCol = cursorColBase + 1 // leading pad
		if searchCursorCol <= 0 {
			searchCursorCol = displayWidth(" " + searchDisplay)
		}
		if p.width > 0 && searchCursorCol > p.width {
			searchCursorCol = p.width
		}
	}
	if searchCursorRow > 0 && p.searchMode {
		p.writeString("\x1b[?25h")
		if searchCursorCol < 1 {
			searchCursorCol = 1
		}
		p.printf("\x1b[%d;%dH", searchCursorRow, searchCursorCol)
	} else {
		p.writeString("\x1b[?25l")
	}

	if p.writer != nil {
		return p.writer.Flush()
	}
	return nil
}

func (p *PreviewPager) renderHelpOverlay() error {
	p.writeString("\x1b[?25l")
	p.writeString("\x1b[2J")
	p.writeString("\x1b[H")

	row := 1
	p.drawStyledRow(row, " Pager shortcuts ", true, headerBarStyle)
	row++

	helpLines := p.helpOverlayLines()
	maxRow := p.height - 1
	if maxRow < row {
		maxRow = row
	}
	for _, line := range helpLines {
		if row >= maxRow {
			break
		}
		p.drawRow(row, line, false)
		row++
	}
	for row < maxRow {
		p.drawRow(row, "", false)
		row++
	}

	p.drawStatus("j/k/PgUp/PgDn scroll  ·  q/Esc/← close")

	if p.writer != nil {
		return p.writer.Flush()
	}
	return nil
}

func (p *PreviewPager) drawRow(row int, text string, bold bool) {
	if row < 1 {
		row = 1
	}
	if row > p.height {
		return
	}

	p.printf("\x1b[%d;1H", row)
	p.writeString("\x1b[0m\x1b[2K")
	if bold {
		p.writeString("\x1b[1m")
	}

	renderText := text
	if !p.wrapEnabled && p.width > 0 {
		renderText = truncateToWidth(text, p.width)
	}
	p.writeString(renderText)

	if bold {
		p.writeString("\x1b[22m")
	}
}

func (p *PreviewPager) reflowMarkdownFormatted() {
	if p.state == nil || p.state.PreviewData == nil {
		return
	}
	preview := p.state.PreviewData
	if preview.FormattedKind != "markdown" || len(preview.TextLines) == 0 || p.width <= 0 {
		return
	}
	maxLines := 3
	if p.wrapEnabled {
		maxLines = 0
	}
	segments, meta := statepkg.FormatMarkdownPreview(preview.TextLines, p.width, maxLines, p.wrapEnabled)
	if len(segments) == 0 || len(meta) != len(segments) {
		return
	}

	formatted := make([]string, len(segments))
	widths := make([]int, len(segments))
	rules := make([]bool, len(segments))
	styles := make([]string, len(segments))
	for i, line := range segments {
		txt, isRule, style := ansiFromSegments(line)
		formatted[i] = txt
		rules[i] = isRule
		styles[i] = style
		if i < len(meta) && meta[i].DisplayWidth > 0 {
			widths[i] = meta[i].DisplayWidth
		} else {
			widths[i] = segmentDisplayWidth(line)
		}
	}

	p.formattedLines = formatted
	p.formattedWidths = widths
	p.formattedRules = rules
	p.formattedStyles = styles

	if p.showFormatted {
		p.lines = p.formattedLines
		p.lineWidths = p.formattedWidths
		p.rowSpans = nil
		p.rowPrefix = nil
	}
}

func (p *PreviewPager) drawStyledRow(row int, text string, bold bool, style string) {
	if row < 1 {
		row = 1
	}
	if row > p.height {
		return
	}

	p.printf("\x1b[%d;1H", row)
	p.writeString("\x1b[0m\x1b[2K")

	if style != "" {
		p.writeString(style)
	}
	if bold {
		p.writeString("\x1b[1m")
	}

	renderText := text
	available := p.width
	if available < 0 {
		available = 0
	}

	needsPadding := style == headerBarStyle
	if needsPadding && available >= 2 {
		bodyWidth := available - 2
		clipped, truncated := clipTextToWidth(renderText, bodyWidth)
		renderText = clipped
		p.writeString(" " + renderText)
		renderWidth := displayWidth(renderText)
		padding := bodyWidth - renderWidth
		if padding > 0 {
			p.writeString(strings.Repeat(" ", padding))
		}
		if truncated {
			p.writeString("…")
		} else {
			p.writeString(" ")
		}
	} else {
		if available > 0 {
			renderText = truncateToWidth(renderText, available)
		}
		p.writeString(renderText)
	}

	if bold {
		p.writeString("\x1b[22m")
	}
	if style != "" || bold {
		p.writeString("\x1b[0m")
	}
}

func (p *PreviewPager) drawStatus(text string) {
	if p.height < 1 {
		return
	}
	display := " " + text + " "
	if p.width > 0 && displayWidth(display) > p.width {
		display = truncateToWidth(display, p.width)
	}
	style := statusBarStyle
	if strings.TrimSpace(p.statusMessage) != "" && p.statusStyle != "" {
		style = p.statusStyle
	}
	p.drawStyledRow(p.height, display, false, style)
}

func (p *PreviewPager) statusLine(totalLines, visible, charCount int, search string) string {
	lineApprox := p.isLineCountApprox()
	charApprox := p.isCharCountApprox()
	kind := p.contentKind()

	segments := []string{p.positionSegment(totalLines, visible, lineApprox)}
	if count := p.countSegment(kind, charCount, charApprox); count != "" {
		segments = append(segments, count)
	}
	if offset := p.binaryOffsetSegment(); offset != "" {
		segments = append(segments, offset)
	}
	segments = append(segments, p.statusBadges(kind)...)
	if search != "" {
		segments = append([]string{search}, segments...)
	}
	segments = filterEmptyStrings(segments)

	base := strings.Join(segments, "  ")
	help := strings.Join(p.helpSegments(), "  ")

	if msg := strings.TrimSpace(p.statusMessage); msg != "" {
		msg = textutil.SanitizeTerminalText(msg)
		if base != "" {
			msg += "  " + base
		}
		if help != "" {
			msg += "  " + help
		}
		return msg
	}

	if help != "" {
		if base != "" {
			base += "  "
		}
		base += help
	}
	return base
}

func (p *PreviewPager) positionSegment(totalLines, visible int, approx bool) string {
	if visible < 1 {
		visible = 1
	}
	if p.wrapEnabled {
		totalRows := p.totalRowCount()
		startRow := 0
		if totalRows > 0 {
			startRow = p.currentRowNumber() + 1
			if startRow > totalRows {
				startRow = totalRows
			}
		}
		endRow := startRow + visible - 1
		if endRow > totalRows {
			endRow = totalRows
		}
		percent := p.progressPercent(startRow, totalRows)
		linesText := fmt.Sprintf("%d lines", totalLines)
		if approx {
			linesText = fmt.Sprintf("~%s", linesText)
		}
		return fmt.Sprintf("%d-%d/%d rows (%s, %d%%)", startRow, endRow, totalRows, linesText, percent)
	}

	lineLabel := "lines"
	if p.binaryMode {
		lineLabel = "rows"
	}
	start := 0
	if totalLines > 0 {
		start = p.state.PreviewScrollOffset + 1
		if start > totalLines {
			start = totalLines
		}
	}
	end := start + visible - 1
	if end > totalLines {
		end = totalLines
	}
	percent := p.progressPercent(start, totalLines)
	linesText := fmt.Sprintf("%d-%d/%d %s", start, end, totalLines, lineLabel)
	if approx && !p.binaryMode {
		linesText = fmt.Sprintf("%d-%d/~%d %s", start, end, totalLines, lineLabel)
	}
	return fmt.Sprintf("%s (%d%%)", linesText, percent)
}

func (p *PreviewPager) countSegment(kind pagerContentKind, charCount int, approx bool) string {
	if charCount <= 0 {
		return ""
	}
	prefix := ""
	if approx {
		prefix = "~"
	}
	if kind == pagerContentBinary {
		return fmt.Sprintf("%s%d bytes", prefix, charCount)
	}
	return fmt.Sprintf("%s%d chars", prefix, charCount)
}

func (p *PreviewPager) binaryOffsetSegment() string {
	if p == nil || !p.binaryMode || p.state == nil {
		return ""
	}
	bytesPerLine := binaryPreviewLineWidth
	totalBytes := int64(0)
	if p.binarySource != nil {
		if p.binarySource.bytesPerLine > 0 {
			bytesPerLine = p.binarySource.bytesPerLine
		}
		totalBytes = p.binarySource.totalBytes
	}
	if totalBytes == 0 && p.state.PreviewData != nil {
		totalBytes = p.state.PreviewData.BinaryInfo.TotalBytes
		if totalBytes == 0 {
			totalBytes = p.state.PreviewData.Size
		}
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	offset := int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
	if totalBytes <= 0 {
		return fmt.Sprintf("offset: %s", formatHexOffset(offset))
	}
	percent := p.binaryProgressPercent(offset, totalBytes)
	return fmt.Sprintf("offset: %s/%s  pos: %d%%", formatHexOffset(offset), formatHexOffset(totalBytes), percent)
}

func (p *PreviewPager) statusBadges(kind pagerContentKind) []string {
	preview := (*statepkg.PreviewData)(nil)
	if p.state != nil {
		preview = p.state.PreviewData
	}
	badges := []string{}
	if label := contentKindLabel(kind); label != "" {
		badges = append(badges, "type:"+label)
	}
	if !p.binaryMode {
		wrap := "off"
		if p.wrapEnabled {
			wrap = "on"
		}
		badges = append(badges, "wrap:"+wrap)
	}
	formattedAvailable := len(p.formattedLines) > 0
	formattedReason := preview != nil && preview.FormattedUnavailableReason != ""
	if formattedAvailable || formattedReason {
		mode := "raw"
		if formattedAvailable && p.showFormatted {
			mode = "pretty"
		} else if formattedReason && !formattedAvailable {
			mode = "raw*"
		}
		badges = append(badges, "fmt:"+mode)
	}
	if preview != nil && preview.HiddenFormattingDetected && !p.binaryMode {
		badges = append(badges, "hidden:yes")
	}
	infoState := "off"
	if p.showInfo {
		infoState = "on"
	}
	badges = append(badges, "info:"+infoState)
	if p.binaryMode && p.effectiveBinaryFullScan() {
		badges = append(badges, "scan:*")
	}
	return badges
}

func (p *PreviewPager) helpOverlayLines() []string {
	width := p.width
	if width <= 0 {
		width = 80
	}
	available := width
	if available > 2 {
		available -= 2
	}
	lines := []string{}
	useSeparator := width >= 60
	separator := helpSeparator(available)
	for _, section := range p.helpSections() {
		if len(section.entries) == 0 {
			continue
		}
		if len(lines) > 0 {
			if useSeparator && separator != "" {
				lines = append(lines, separator)
			} else {
				lines = append(lines, "")
			}
		}
		lines = append(lines, section.title+":")
		lines = append(lines, formatHelpEntries(section.entries, available)...)
	}
	return lines
}

func helpSeparator(width int) string {
	if width < 4 {
		return ""
	}
	maxWidth := width
	if maxWidth > 60 {
		maxWidth = 60
	}
	return strings.Repeat("-", maxWidth)
}

func formatHelpEntries(entries []helpEntry, width int) []string {
	maxKeys := 0
	for _, entry := range entries {
		if w := displayWidth(entry.keys); w > maxKeys {
			maxKeys = w
		}
	}
	padding := maxKeys + 2
	if padding < 2 {
		padding = 2
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line := fmt.Sprintf("  %-*s %s", padding, entry.keys, entry.desc)
		if width > 0 && displayWidth(line) > width {
			line = truncateToWidth(line, width)
		}
		lines = append(lines, line)
	}
	return lines
}

func (p *PreviewPager) helpSections() []helpSection {
	nav := []helpEntry{
		{keys: "↑/↓ or j/k", desc: "Scroll one line"},
		{keys: "Shift+↑/↓", desc: "Jump ±10 lines"},
		{keys: "PgUp / b", desc: "Page up"},
		{keys: "PgDn / space", desc: "Page down"},
		{keys: "Home/End or g/G", desc: "Jump to start/end"},
	}
	if p.binaryMode {
		nav = append(nav,
			helpEntry{keys: "[ / ]", desc: "Jump ±4 KB"},
			helpEntry{keys: "{ / }", desc: "Jump ±64 KB"},
		)
	} else if p.wrapEnabled {
		nav = append(nav, helpEntry{keys: "[ / ]", desc: "Skip wrapped line"})
	}

	view := []helpEntry{
		{keys: "?", desc: "Toggle this help"},
		{keys: "i", desc: "Toggle info line"},
	}
	if !p.binaryMode {
		view = append(view, helpEntry{keys: "w or →", desc: "Toggle wrap"})
	}
	if len(p.formattedLines) > 0 {
		view = append(view, helpEntry{keys: "f", desc: "Toggle formatted view"})
	}

	actions := []helpEntry{}
	if p.clipboardAvailable() {
		actions = append(actions,
			helpEntry{keys: "c", desc: "Copy visible lines"},
			helpEntry{keys: "C", desc: "Copy entire file (raw)"},
		)
	}
	if p.canOpenEditor() {
		actions = append(actions, helpEntry{keys: "e", desc: "Open in editor"})
	}
	actions = append(actions, helpEntry{keys: "Ctrl+C", desc: "Quit immediately"})

	exit := []helpEntry{
		{keys: "← / q / x / Esc", desc: "Exit pager"},
	}

	search := []helpEntry{
		{keys: "/", desc: "Enter search"},
		{keys: "n / N", desc: "Jump to next/prev hit"},
	}
	if p.binaryMode {
		search = append(search, helpEntry{keys: ":", desc: "Enter binary search"})
		search = append(search, helpEntry{keys: "Ctrl+B", desc: "Toggle text/hex mode while searching"})
		search = append(search, helpEntry{keys: "Ctrl+L", desc: "Toggle full scan for binary search"})
	}

	sections := []helpSection{
		{title: "Navigation", entries: nav},
		{title: "View", entries: view},
	}
	if len(search) > 0 {
		sections = append(sections, helpSection{title: "Search", entries: search})
	}
	sections = append(sections,
		helpSection{title: "Actions", entries: actions},
		helpSection{title: "Exit", entries: exit},
	)
	return sections
}

func (p *PreviewPager) helpSegments() []string {
	return []string{"? help"}
}

type helpEntry struct {
	keys string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func (p *PreviewPager) headerLines() []string {
	if p.state == nil || p.state.PreviewData == nil {
		return []string{"(no preview available)"}
	}
	preview := p.state.PreviewData
	fullPath := filepath.Join(p.state.CurrentPath, preview.Name)

	lines := []string{textutil.SanitizeTerminalText(fullPath)}
	if p.showInfo {
		if info := p.infoLine(preview); info != "" {
			lines = append(lines, info)
		}
	}
	return lines
}

func (p *PreviewPager) infoLine(preview *statepkg.PreviewData) string {
	segments := p.infoSegments(preview)
	if len(segments) == 0 {
		return ""
	}
	return textutil.SanitizeTerminalText(strings.Join(segments, "  •  "))
}

func (p *PreviewPager) infoSegments(preview *statepkg.PreviewData) []string {
	if preview == nil {
		return nil
	}
	meta := fmt.Sprintf("%s  %s  %s", preview.Mode.String(), formatSize(preview.Size), preview.Modified.Format("2006-01-02 15:04:05"))
	segments := []string{meta}
	segments = append(segments, p.detailInfoSegments(preview)...)
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		out = append(out, seg)
	}
	return out
}

func (p *PreviewPager) detailInfoSegments(preview *statepkg.PreviewData) []string {
	kind := p.contentKind()
	segments := []string{}
	if label := contentKindLabel(kind); label != "" {
		segments = append(segments, "type:"+label)
	}
	switch kind {
	case pagerContentBinary:
		if preview.BinaryInfo.TotalBytes > 0 {
			segments = append(segments, fmt.Sprintf("bytes:%d", preview.BinaryInfo.TotalBytes))
		}
		if preview.LineCount > 0 {
			segments = append(segments, fmt.Sprintf("rows:%d", preview.LineCount))
		}
		segments = append(segments, fmt.Sprintf("%d B/row", binaryPreviewLineWidth))
	default:
		if lineCount := p.headerLineCount(preview); lineCount > 0 {
			lineSegment := fmt.Sprintf("lines:%d", lineCount)
			if p.isLineCountApprox() {
				lineSegment = fmt.Sprintf("lines:~%d", lineCount)
			}
			segments = append(segments, lineSegment)
		}
		if count := p.headerCharCount(preview); count > 0 {
			label := fmt.Sprintf("chars:%d", count)
			if p.isCharCountApprox() {
				label = fmt.Sprintf("chars:~%d", count)
			}
			segments = append(segments, label)
		}
		if enc := formatEncodingLabel(preview.TextEncoding); enc != "" {
			segments = append(segments, "encoding:"+enc)
		}
		if p.rawTextSource != nil {
			if !p.rawTextSource.FullyLoaded() {
				segments = append(segments, "streaming from disk")
			}
		} else if preview.TextTruncated {
			segments = append(segments, "preview truncated")
		}
		if preview.HiddenFormattingDetected {
			segments = append(segments, "hidden formatting detected")
		}
		if preview.FormattedUnavailableReason != "" {
			segments = append(segments, preview.FormattedUnavailableReason)
		}
	}
	return segments
}

func formatEncodingLabel(enc fsutil.UnicodeEncoding) string {
	switch enc {
	case fsutil.EncodingUnknown:
		return "utf-8/ascii"
	case fsutil.EncodingUTF8BOM:
		return "utf-8 bom"
	case fsutil.EncodingUTF16LE:
		return "utf-16le"
	case fsutil.EncodingUTF16BE:
		return "utf-16be"
	default:
		return ""
	}
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansiDisplayWidth(text) <= width {
		return text
	}
	truncated, _ := ansiTruncate(text, width, true)
	return truncated
}

func clipTextToWidth(text string, width int) (string, bool) {
	if width <= 0 {
		return "", ansiDisplayWidth(text) > 0
	}
	return ansiTruncate(text, width, false)
}

func displayWidth(text string) int {
	return ansiDisplayWidth(text)
}

func ansiFromSegments(segments []statepkg.StyledTextSegment) (string, bool, string) {
	if len(segments) == 0 {
		return "", false, ""
	}
	rule := true
	for _, seg := range segments {
		if seg.Style != statepkg.TextStyleRule {
			rule = false
			break
		}
	}

	var b strings.Builder
	reset := "\x1b[0m"
	var styleCode string
	for _, seg := range segments {
		text := textutil.SanitizeTerminalText(seg.Text)
		if text == "" {
			continue
		}
		code := ansiForStyle(seg.Style)
		if code != "" {
			b.WriteString(code)
			if styleCode == "" {
				styleCode = code
			}
		}
		b.WriteString(text)
		if seg.Style != statepkg.TextStylePlain {
			b.WriteString(reset)
		}
	}
	return b.String(), rule, styleCode
}

func ansiForStyle(kind statepkg.TextStyleKind) string {
	switch kind {
	case statepkg.TextStyleStrong, statepkg.TextStyleHeading:
		return "\x1b[1m"
	case statepkg.TextStyleEmphasis:
		return "\x1b[3m"
	case statepkg.TextStyleStrike:
		return "\x1b[9m"
	case statepkg.TextStyleCode:
		return ansiColorSequence(pagerTheme.CodeFg, pagerTheme.CodeBg)
	case statepkg.TextStyleCodeBlock:
		return ansiColorSequence(pagerTheme.CodeBlockFg, pagerTheme.CodeBlockBg)
	case statepkg.TextStyleLink:
		return "\x1b[4m"
	case statepkg.TextStyleRule:
		return "\x1b[2m"
	default:
		return ""
	}
}

var pagerTheme = renderpkg.GetColorTheme()

func ansiColorSequence(fg, bg tcell.Color) string {
	if fg == tcell.ColorDefault && bg == tcell.ColorDefault {
		return ""
	}
	parts := make([]string, 0, 2)
	if fg != tcell.ColorDefault {
		r, g, b := fg.RGB()
		parts = append(parts, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if bg != tcell.ColorDefault {
		r, g, b := bg.RGB()
		parts = append(parts, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func segmentDisplayWidth(segments []statepkg.StyledTextSegment) int {
	width := 0
	for _, seg := range segments {
		width += displayWidth(seg.Text)
	}
	return width
}

func ansiDisplayWidth(text string) int {
	width := 0
	for len(text) > 0 {
		esc := strings.IndexByte(text, '\x1b')
		if esc == -1 {
			width += textutil.DisplayWidth(text)
			break
		}
		if esc > 0 {
			width += textutil.DisplayWidth(text[:esc])
		}
		text = text[esc:]
		if len(text) >= 2 && text[0] == '\x1b' && text[1] == '[' {
			end := 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			if end > len(text) {
				end = len(text)
			}
			text = text[end:]
			continue
		}
		text = text[1:]
	}
	return width
}

func ansiTruncate(text string, width int, withEllipsis bool) (string, bool) {
	if width <= 0 {
		return "", ansiDisplayWidth(text) > 0
	}
	const ellipsisRune = '…'
	ellipsisWidth := textutil.DisplayWidth(string(ellipsisRune))
	if ellipsisWidth <= 0 {
		ellipsisWidth = 1
	}
	target := width
	if withEllipsis {
		target -= ellipsisWidth
	}
	if target < 0 {
		target = 0
	}

	var b strings.Builder
	b.Grow(len(text))
	consumed := 0
	truncated := false

	for len(text) > 0 {
		if text[0] == '\x1b' && len(text) > 1 && text[1] == '[' {
			end := 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				end++
			}
			if end > len(text) {
				end = len(text)
			}
			b.WriteString(text[:end])
			text = text[end:]
			continue
		}

		g := uniseg.NewGraphemes(text)
		if !g.Next() {
			break
		}
		cluster := g.Str()
		clusterWidth := textutil.DisplayWidth(cluster)
		if clusterWidth <= 0 {
			clusterWidth = 1
		}
		if consumed+clusterWidth > target {
			truncated = true
			break
		}
		b.WriteString(cluster)
		consumed += clusterWidth
		text = text[len(cluster):]
	}

	if !truncated && len(text) > 0 {
		truncated = true
	}

	if truncated && withEllipsis && ellipsisWidth <= width {
		b.WriteRune(ellipsisRune)
	}
	return b.String(), truncated
}
