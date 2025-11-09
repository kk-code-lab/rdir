package render

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

// Renderer handles all UI rendering
type Renderer struct {
	screen           tcell.Screen
	theme            ColorTheme
	runeWidthCache   [128]int // ASCII cache (0-127)
	runeWidthCacheMu sync.RWMutex
	runeWidthWide    sync.Map // For non-ASCII runes
}

// NewRenderer creates a new renderer
func NewRenderer(screen tcell.Screen) *Renderer {
	return &Renderer{
		screen: screen,
		theme:  GetColorTheme(),
	}
}

// Render draws the entire UI based on state
func (r *Renderer) Render(state *statepkg.AppState) {
	r.screen.Clear()

	w, h := r.screen.Size()

	layout := r.computeLayout(w, state)

	// Draw all panels
	r.drawHeader(state, w, h)
	if layout.sidebarWidth > 0 {
		r.drawSidebar(state, layout.sidebarWidth, h)
		// Fill separator column
		if layout.sideSeparatorWidth > 0 && layout.sidebarWidth < w {
			for y := 1; y < h-1; y++ {
				r.screen.SetContent(layout.sidebarWidth, y, ' ', nil, tcell.StyleDefault)
			}
		}
	}
	r.drawMainPanel(state, layout.mainPanelStart, layout.mainPanelWidth, h)
	if layout.showPreview {
		if layout.contentSeparatorWidth > 0 && layout.previewStart-layout.contentSeparatorWidth >= 0 {
			sepX := layout.previewStart - layout.contentSeparatorWidth
			for y := 1; y < h-1; y++ {
				r.screen.SetContent(sepX, y, ' ', nil, tcell.StyleDefault)
			}
		}
		r.drawPreviewPanel(state, layout, w, h)
	}
	r.drawStatusLine(state, w, h)

	r.screen.Show()
}

// drawHeader renders the top bar with title and breadcrumb
func (r *Renderer) drawHeader(state *statepkg.AppState, w, h int) {
	headerText := "rdir"
	headerStyle := tcell.StyleDefault.Background(r.theme.FooterBg).Foreground(r.theme.FooterFg)

	endX := r.drawTextLine(0, 0, w, headerText, headerStyle)
	currentPath := state.CurrentPath
	if currentPath == "" {
		currentPath = "/"
	}

	if endX < w {
		r.screen.SetContent(endX, 0, ' ', nil, headerStyle)
		endX++
	}

	if endX < w {
		available := w - endX
		segments := r.formatBreadcrumbSegments(currentPath)
		if len(segments) > 0 {
			lastIdx := len(segments) - 1
			if lastIdx > 0 {
				prefix := strings.Join(segments[:lastIdx], " › ")
				prefix = r.fitBreadcrumb(prefix, available)
				endX = r.drawTextLine(endX, 0, available, prefix, headerStyle)
				if endX < w {
					sep := r.fitBreadcrumb(" › ", w-endX)
					endX = r.drawTextLine(endX, 0, w-endX, sep, headerStyle)
				}
			}

			if endX < w {
				lastSegment := r.fitBreadcrumb(segments[lastIdx], w-endX)
				highlightStyle := headerStyle.Bold(true)
				endX = r.drawTextLine(endX, 0, w-endX, lastSegment, highlightStyle)
			}
		}
	}

	// Fill remaining space
	for x := endX; x < w; x++ {
		r.screen.SetContent(x, 0, ' ', nil, headerStyle)
	}
}

// fitBreadcrumb trims the breadcrumb path to fit within the available width
func (r *Renderer) fitBreadcrumb(path string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(path)
	totalWidth := 0
	for _, ru := range runes {
		ruWidth := r.cachedRuneWidth(ru)
		if ruWidth < 0 {
			ruWidth = 0
		}
		totalWidth += ruWidth
		if totalWidth > width {
			break
		}
	}

	if totalWidth <= width {
		return path
	}

	ellipsis := "…"
	ellipsisWidth := r.cachedRuneWidth('…')
	if ellipsisWidth < 0 {
		ellipsisWidth = 1
	}
	if width <= ellipsisWidth {
		return ellipsis
	}

	available := width - ellipsisWidth

	// Trim from the left, keep end of the path (most useful part)
	resultRunes := []rune{}
	currentWidth := 0
	for i := len(runes) - 1; i >= 0; i-- {
		ruWidth := r.cachedRuneWidth(runes[i])
		if ruWidth < 0 {
			ruWidth = 0
		}
		if currentWidth+ruWidth > available {
			break
		}
		resultRunes = append([]rune{runes[i]}, resultRunes...)
		currentWidth += ruWidth
	}

	return ellipsis + string(resultRunes)
}

func (r *Renderer) formatBreadcrumbSegments(path string) []string {
	if path == "" {
		return []string{"/"}
	}

	cleanPath := filepath.Clean(path)
	if cleanPath == "." {
		cleanPath = "/"
	}

	slashed := filepath.ToSlash(cleanPath)
	if slashed == "/" {
		return []string{"/"}
	}

	var segments []string

	if strings.HasPrefix(slashed, "/") {
		segments = append(segments, "/")
		slashed = strings.TrimPrefix(slashed, "/")
	}

	for _, part := range strings.Split(slashed, "/") {
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}

	if len(segments) == 0 {
		return []string{cleanPath}
	}

	return segments
}

// drawStatusLine renders the status line at the bottom with path and help text
func (r *Renderer) drawStatusLine(state *statepkg.AppState, w, h int) {
	normalStyle := tcell.StyleDefault.Background(r.theme.FooterBg).Foreground(r.theme.FooterFg)
	flashStyle := tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack)

	// Check if we should flash (within 0.1 seconds of last yank)
	isFlashing := false
	if !state.LastYankTime.IsZero() {
		elapsed := time.Since(state.LastYankTime)
		isFlashing = elapsed < 100*time.Millisecond
	}

	var pathText string

	// In global search mode, show the full path of the selected search result
	if state.GlobalSearchActive && state.GlobalSearchIndex >= 0 && state.GlobalSearchIndex < len(state.GlobalSearchResults) {
		result := state.GlobalSearchResults[state.GlobalSearchIndex]
		pathText = result.FilePath
	} else {
		pathText = state.CurrentFilePath()
	}

	// Add symlink target info if applicable
	symlinkTarget := state.SymlinkTarget()
	if symlinkTarget != "" && !state.GlobalSearchActive {
		pathText = pathText + " → " + symlinkTarget
	}

	pathRunes := []rune(pathText)

	// Calculate how many lines we need for the path (accounting for wide characters)
	pathWidth := 0
	pathLines := 1
	for _, ru := range pathRunes {
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth < 0 {
			runeWidth = 0
		}
		if pathWidth+runeWidth > w {
			pathLines++
			pathWidth = runeWidth
		} else {
			pathWidth += runeWidth
		}
	}
	if pathLines < 1 {
		pathLines = 1
	}

	// Status line occupies: pathLines + 1 (for help text)
	statusLineHeight := pathLines + 1
	startY := h - statusLineHeight

	// Draw path lines with wrapping
	pathStyle := normalStyle
	if isFlashing {
		pathStyle = flashStyle
	}

	x := 0
	y := startY
	for _, ru := range pathRunes {
		if x >= w {
			y++
			x = 0
		}
		if y < h {
			r.screen.SetContent(x, y, ru, nil, pathStyle)
		}
		// Move x by actual rune width, accounting for wide characters
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth < 0 {
			runeWidth = 0
		}
		x += runeWidth
	}

	// Fill remaining spaces on last path line
	for i := x; i < w && y < h; i++ {
		r.screen.SetContent(i, y, ' ', nil, pathStyle)
	}

	// Draw help text on last line
	helpText := buildFooterHelpText(state)
	if helpText == "" {
		helpText = " "
	}
	if indexLine := formatIndexStatusLine(state.CurrentIndexStatus()); indexLine != "" {
		helpText = fmt.Sprintf("%s | %s", helpText, indexLine)
	}

	helpY := h - 1
	x = 0
	for _, ru := range helpText {
		if x >= w {
			break
		}
		r.screen.SetContent(x, helpY, ru, nil, normalStyle)
		// Move x by actual rune width, accounting for wide characters
		runeWidth := r.cachedRuneWidth(ru)
		if runeWidth < 0 {
			runeWidth = 0
		}
		x += runeWidth
	}
	// Fill remaining spaces
	for x < w {
		r.screen.SetContent(x, helpY, ' ', nil, normalStyle)
		x++
	}
}

// drawSidebar renders the left sidebar with entries from the parent directory
func (r *Renderer) drawSidebar(state *statepkg.AppState, sidebarWidth, h int) {
	baseBgStyle := tcell.StyleDefault.Background(r.theme.SidebarBg).Foreground(r.theme.SidebarFg)

	parentPath := filepath.Dir(state.CurrentPath)
	hasParent := parentPath != "" && parentPath != state.CurrentPath
	currentName := filepath.Base(state.CurrentPath)
	if currentName == "" {
		currentName = state.CurrentPath
	}

	y := 1
	entries := state.ParentEntries
	if !hasParent {
		if y < h-1 {
			placeholder := " No parent directory"
			endX := r.drawTextLine(0, y, sidebarWidth, placeholder, baseBgStyle)
			for x := endX; x < sidebarWidth; x++ {
				r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
			}
			y++
		}
	} else if len(entries) == 0 {
		if y < h-1 {
			placeholder := " Parent is empty"
			endX := r.drawTextLine(0, y, sidebarWidth, placeholder, baseBgStyle)
			for x := endX; x < sidebarWidth; x++ {
				r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
			}
			y++
		}
	} else {
		maxRows := h - 2
		if maxRows < 1 {
			maxRows = 1
		}

		currentIdx := 0
		foundCurrent := false
		for idx, entry := range entries {
			if entry.Name == currentName {
				currentIdx = idx
				foundCurrent = true
				break
			}
		}
		if !foundCurrent {
			currentIdx = 0
		}

		startIdx := 0
		if len(entries) > maxRows {
			startIdx = currentIdx - maxRows/2
			if startIdx < 0 {
				startIdx = 0
			}
			if startIdx > len(entries)-maxRows {
				startIdx = len(entries) - maxRows
			}
		}

		endIdx := len(entries)
		if endIdx-startIdx > maxRows {
			endIdx = startIdx + maxRows
		}

		for i := startIdx; i < endIdx; i++ {
			entry := entries[i]
			if y >= h-1 {
				break
			}

			rowStyle := baseBgStyle
			if entry.Name == currentName {
				rowStyle = tcell.StyleDefault.Background(r.theme.SidebarActiveBg).Foreground(r.theme.SidebarActiveFg)
			} else if entry.IsSymlink {
				rowStyle = baseBgStyle.Foreground(r.theme.SymlinkFg)
			} else if entry.IsDir {
				rowStyle = baseBgStyle.Foreground(r.theme.DirectoryFg)
			}

			icon := " "
			if entry.IsSymlink {
				icon = "@"
			} else if entry.IsDir {
				icon = "/"
			}

			prefix := fmt.Sprintf(" %s ", icon)
			nameWidth := sidebarWidth - r.measureTextWidth(prefix)
			displayName := entry.Name
			if nameWidth > 0 {
				displayName = r.truncateTextToWidth(displayName, nameWidth)
			} else {
				displayName = ""
			}

			line := prefix + displayName
			endX := r.drawTextLine(0, y, sidebarWidth, line, rowStyle)
			for x := endX; x < sidebarWidth; x++ {
				r.screen.SetContent(x, y, ' ', nil, rowStyle)
			}

			y++
		}
	}

	// Fill rest with empty space
	for y < h-1 {
		for x := 0; x < sidebarWidth; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
		}
		y++
	}
}

// drawMainPanel renders the file list
func (r *Renderer) drawMainPanel(state *statepkg.AppState, startX, panelWidth, h int) {
	baseBgStyle := tcell.StyleDefault.Background(r.theme.SidebarBg)

	// Draw header with current directory, filter, or global search (only when needed)
	headerStyle := baseBgStyle.Foreground(r.theme.SidebarFg)
	hasHeader := false
	contentStartY := 1

	if state.GlobalSearchActive {
		hasHeader = true

		cursor := state.GlobalSearchCursorPos
		queryRunes := []rune(state.GlobalSearchQuery)
		if cursor < 0 {
			cursor = 0
		}
		if cursor > len(queryRunes) {
			cursor = len(queryRunes)
		}

		highlightStyle := headerStyle.Background(r.theme.SelectionBg).Foreground(r.theme.SelectionFg)
		placeholderStyle := headerStyle.Dim(true)

		x := startX
		y := 1
		maxX := startX + panelWidth

		for _, ru := range "> " {
			if x >= maxX {
				break
			}
			x = r.drawStyledRune(x, y, maxX, ru, headerStyle)
		}

		if len(queryRunes) == 0 {
			if x < maxX {
				x = r.drawStyledRune(x, y, maxX, '█', highlightStyle)
			}
			for _, ru := range "(type to search)" {
				if x >= maxX {
					break
				}
				x = r.drawStyledRune(x, y, maxX, ru, placeholderStyle)
			}
		} else {
			highlightIndex := -1
			if cursor < len(queryRunes) {
				highlightIndex = cursor
			}

			for idx, ru := range queryRunes {
				if x >= maxX {
					break
				}
				style := headerStyle
				if idx == highlightIndex {
					style = highlightStyle
				}
				x = r.drawStyledRune(x, y, maxX, ru, style)
			}

			if cursor == len(queryRunes) && x < maxX {
				x = r.drawStyledRune(x, y, maxX, '█', highlightStyle)
			}
		}

		status := formatSearchHeaderStatus(state, state.CurrentIndexStatus())
		if status != "" && x < maxX {
			for _, ru := range "  — " + status {
				if x >= maxX {
					break
				}
				x = r.drawStyledRune(x, y, maxX, ru, headerStyle)
			}
		}

		for x < maxX {
			x = r.drawStyledRune(x, y, maxX, ' ', headerStyle)
		}
	} else if state.FilterActive {
		headerText := "/" + state.FilterQuery
		endX := r.drawTextLine(startX, 1, panelWidth, headerText, headerStyle)

		cursorStyle := headerStyle.Background(r.theme.SelectionBg).Foreground(r.theme.SelectionFg)
		if endX < startX+panelWidth {
			endX = r.drawStyledRune(endX, 1, startX+panelWidth, '█', cursorStyle)
		}
		for x := endX; x < startX+panelWidth; x++ {
			r.screen.SetContent(x, 1, ' ', nil, headerStyle)
		}
		hasHeader = true
	}

	if hasHeader {
		contentStartY = 2
	}

	// Draw file list or global search results
	if state.GlobalSearchActive {
		r.drawGlobalSearchResults(state, startX, panelWidth, h, contentStartY, baseBgStyle)
	} else {
		r.drawFileList(state, startX, panelWidth, h, contentStartY, baseBgStyle)
	}
}

// drawFileList renders the normal file list
func (r *Renderer) drawFileList(state *statepkg.AppState, startX, panelWidth, h int, listStartY int, baseBgStyle tcell.Style) {
	// Draw file list
	displayFiles := state.DisplayFiles()
	bottomLimit := h - 2
	if listStartY >= bottomLimit {
		listStartY = bottomLimit - 1
	}
	visibleLines := bottomLimit - listStartY
	if visibleLines < 0 {
		visibleLines = 0
	}

	endIndex := state.ScrollOffset + visibleLines
	if endIndex > len(displayFiles) {
		endIndex = len(displayFiles)
	}

	displayY := listStartY
	for displayIdx := state.ScrollOffset; displayIdx < endIndex; displayIdx++ {
		if displayY >= bottomLimit {
			break
		}

		f := displayFiles[displayIdx]

		// Get actual file index for selection comparison (testable logic in state.go)
		actualIdx := state.ActualIndexFromDisplayIndex(displayIdx)

		// Highlight selected row
		var rowStyle tcell.Style
		if actualIdx == state.SelectedIndex {
			rowStyle = tcell.StyleDefault.Background(r.theme.SelectionBg).Foreground(r.theme.SelectionFg)
		} else if f.IsSymlink {
			rowStyle = baseBgStyle.Foreground(r.theme.SymlinkFg)
		} else if f.IsDir {
			rowStyle = baseBgStyle.Foreground(r.theme.DirectoryFg)
		} else {
			rowStyle = baseBgStyle.Foreground(r.theme.FileFg)
		}

		// Icon: @ for symlinks, / for directories, space for files
		icon := " "
		if f.IsSymlink {
			icon = "@"
		} else if f.IsDir {
			icon = "/"
		}

		prefix := fmt.Sprintf(" %s ", icon)
		nameWidth := panelWidth - r.measureTextWidth(prefix)
		displayName := f.Name
		if nameWidth > 0 {
			displayName = r.truncateTextToWidth(displayName, nameWidth)
		} else {
			displayName = ""
		}

		text := prefix + displayName

		// Draw text with proper Unicode handling
		endX := r.drawTextLine(startX, displayY, panelWidth, text, rowStyle)

		// Fill remaining space with padding
		for x := endX; x < startX+panelWidth; x++ {
			r.screen.SetContent(x, displayY, ' ', nil, rowStyle)
		}

		displayY++
	}

	// Fill rest with empty space
	for y := displayY; y < bottomLimit; y++ {
		for x := startX; x < startX+panelWidth; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
		}
	}
}

// drawGlobalSearchResults renders global search results
func (r *Renderer) drawGlobalSearchResults(state *statepkg.AppState, startX, panelWidth, h int, listStartY int, baseBgStyle tcell.Style) {
	// Draw search results
	if len(state.GlobalSearchResults) == 0 {
		// No results to display
		displayY := listStartY
		bottomLimit := h - 2
		if displayY >= bottomLimit {
			displayY = bottomLimit - 1
		}
		for y := displayY; y < bottomLimit; y++ {
			for x := startX; x < startX+panelWidth; x++ {
				r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
			}
		}
		return
	}

	bottomLimit := h - 2
	if listStartY >= bottomLimit {
		listStartY = bottomLimit - 1
	}
	visibleLines := bottomLimit - listStartY
	if visibleLines < 0 {
		visibleLines = 0
	}

	// Clamp GlobalSearchIndex to valid range
	selectedIdx := state.GlobalSearchIndex
	if selectedIdx < 0 {
		selectedIdx = 0
	}
	if selectedIdx >= len(state.GlobalSearchResults) {
		selectedIdx = len(state.GlobalSearchResults) - 1
	}

	// Calculate which results to display using state's scroll offset
	startIdx := state.GlobalSearchScroll
	if startIdx < 0 {
		startIdx = 0
	}
	maxStart := len(state.GlobalSearchResults) - visibleLines
	if maxStart < 0 {
		maxStart = 0
	}
	if startIdx > maxStart {
		startIdx = maxStart
	}

	endIdx := startIdx + visibleLines
	if endIdx > len(state.GlobalSearchResults) {
		endIdx = len(state.GlobalSearchResults)
	}

	displayY := listStartY
	for resultIdx := startIdx; resultIdx < endIdx && resultIdx < len(state.GlobalSearchResults); resultIdx++ {
		if displayY >= bottomLimit {
			break
		}

		result := state.GlobalSearchResults[resultIdx]

		// Highlight selected result
		var rowStyle tcell.Style
		if resultIdx == selectedIdx {
			rowStyle = tcell.StyleDefault.Background(r.theme.SelectionBg).Foreground(r.theme.SelectionFg)
		} else {
			rowStyle = baseBgStyle.Foreground(r.theme.FileFg)
		}

		// Display relative path from root
		relPath, _ := filepath.Rel(state.GlobalSearchRootPath, result.FilePath)

		text := fmt.Sprintf(" %s", relPath)
		text = r.truncateTextToWidth(text, panelWidth)

		// Draw text with proper Unicode handling
		endX := r.drawTextLine(startX, displayY, panelWidth, text, rowStyle)

		// Fill remaining space with padding
		for x := endX; x < startX+panelWidth; x++ {
			r.screen.SetContent(x, displayY, ' ', nil, rowStyle)
		}

		displayY++
	}

	// Fill rest with empty space
	for y := displayY; y < bottomLimit; y++ {
		for x := startX; x < startX+panelWidth; x++ {
			r.screen.SetContent(x, y, ' ', nil, baseBgStyle)
		}
	}
}

// drawPreviewPanel renders the right preview panel
