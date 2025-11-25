package state

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	searchpkg "github.com/kk-code-lab/rdir/internal/search"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
	unicodeenc "golang.org/x/text/encoding/unicode"
)

const (
	// previewByteLimit controls how much of a file we read for previews. Keep it
	// comfortably above formattedPreviewMaxBytes so pretty renderers can work
	// without hitting truncation.
	previewByteLimit         int64 = 256 * 1024
	formattedPreviewMaxBytes       = 128 * 1024
	binaryPreviewMaxBytes          = 1024
	binaryPreviewLineWidth         = 16
	previewDebounceDelay           = 150 * time.Millisecond
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var userHomeDirFn = os.UserHomeDir

type directoryPostLoadFunc func(*StateReducer, *AppState) error

func formatBinaryPreviewLines(content []byte, totalSize int64) BinaryPreview {
	if len(content) == 0 {
		return BinaryPreview{}
	}

	if len(content) > binaryPreviewMaxBytes {
		content = content[:binaryPreviewMaxBytes]
	}

	lines := make([]string, 0, len(content)/binaryPreviewLineWidth+3)

	for offset := 0; offset < len(content); offset += binaryPreviewLineWidth {
		chunk := content[offset:min(offset+binaryPreviewLineWidth, len(content))]
		lines = append(lines, formatHexLine(offset, chunk))
	}

	if int64(len(content)) < totalSize {
		lines = append(lines, fmt.Sprintf("… (%d bytes not shown)", totalSize-int64(len(content))))
	}

	return BinaryPreview{
		Lines:      lines,
		ByteCount:  len(content),
		TotalBytes: totalSize,
	}
}

func formatHexLine(offset int, chunk []byte) string {
	var builder strings.Builder
	builder.Grow(80)
	fmt.Fprintf(&builder, "%08X  ", offset)

	for i := 0; i < binaryPreviewLineWidth; i++ {
		if i < len(chunk) {
			fmt.Fprintf(&builder, "%02X ", chunk[i])
		} else {
			builder.WriteString("   ")
		}
		if i == 7 {
			builder.WriteString(" ")
		}
	}

	builder.WriteString(" |")
	for i := 0; i < len(chunk); i++ {
		builder.WriteByte(printableASCII(chunk[i]))
	}
	for i := len(chunk); i < binaryPreviewLineWidth; i++ {
		builder.WriteByte(' ')
	}
	builder.WriteString("|")
	return builder.String()
}

func printableASCII(b byte) byte {
	if b >= 32 && b <= 126 {
		return b
	}
	return '.'
}

func expandPreviewTextLines(lines []string) ([]string, int) {
	if len(lines) == 0 {
		return nil, 0
	}
	expanded := make([]string, len(lines))
	charCount := 0
	for i, line := range lines {
		text := textutil.ExpandTabs(line, textutil.DefaultTabWidth)
		expanded[i] = text
		charCount += utf8.RuneCountInString(text)
	}
	return expanded, charCount
}

func buildTextPreview(content []byte, truncated bool, encoding fsutil.UnicodeEncoding) ([]string, []TextLineMetadata, int, []byte) {
	if len(content) == 0 {
		return nil, nil, 0, nil
	}

	data := content
	offset := int64(0)
	if encoding == fsutil.EncodingUTF8BOM && len(data) >= 3 {
		data = data[3:]
		offset = 3
	}

	lines := make([]string, 0, 128)
	meta := make([]TextLineMetadata, 0, 128)
	charCount := 0
	cursor := 0

	for cursor < len(data) {
		relative := bytes.IndexByte(data[cursor:], '\n')
		if relative == -1 {
			break
		}
		lineBytes := data[cursor : cursor+relative]
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
			lineBytes = lineBytes[:len(lineBytes)-1]
		}
		text := string(lineBytes)
		expanded := textutil.ExpandTabs(text, textutil.DefaultTabWidth)
		runes := utf8.RuneCountInString(expanded)
		width := textutil.DisplayWidth(expanded)
		startOffset := offset + int64(cursor)
		meta = append(meta, TextLineMetadata{
			Offset:       startOffset,
			Length:       len(lineBytes),
			RuneCount:    runes,
			DisplayWidth: width,
		})
		lines = append(lines, expanded)
		charCount += runes
		cursor += relative + 1
	}

	tail := data[cursor:]
	if !truncated && len(tail) > 0 {
		if len(tail) > 0 && tail[len(tail)-1] == '\r' {
			tail = tail[:len(tail)-1]
		}
		text := string(tail)
		expanded := textutil.ExpandTabs(text, textutil.DefaultTabWidth)
		runes := utf8.RuneCountInString(expanded)
		width := textutil.DisplayWidth(expanded)
		startOffset := offset + int64(cursor)
		meta = append(meta, TextLineMetadata{
			Offset:       startOffset,
			Length:       len(tail),
			RuneCount:    runes,
			DisplayWidth: width,
		})
		lines = append(lines, expanded)
		charCount += runes
		return lines, meta, charCount, nil
	}

	remainder := append([]byte(nil), tail...)
	return lines, meta, charCount, remainder
}

func buildUTF16Preview(content []byte, encoding fsutil.UnicodeEncoding, truncated bool) ([]string, []TextLineMetadata, int, []byte) {
	if len(content) == 0 {
		return nil, nil, 0, nil
	}

	data := content
	offset := int64(0)
	if len(data) >= 2 && (encoding == fsutil.EncodingUTF16LE || encoding == fsutil.EncodingUTF16BE) {
		data = data[2:]
		offset = 2
	}

	lines := make([]string, 0, 128)
	meta := make([]TextLineMetadata, 0, 128)
	charCount := 0
	lineStart := 0

	for lineStart+1 < len(data) {
		lfIndex := -1
		for i := lineStart; i+1 < len(data); i += 2 {
			if isUTF16LF(data[i], data[i+1], encoding) {
				lfIndex = i
				break
			}
		}
		if lfIndex == -1 {
			break
		}
		lineBytes := data[lineStart:lfIndex]
		if len(lineBytes) >= 2 && isUTF16CR(lineBytes[len(lineBytes)-2], lineBytes[len(lineBytes)-1], encoding) {
			lineBytes = lineBytes[:len(lineBytes)-2]
		}
		expanded, runes, width := expandUTF16Line(lineBytes, encoding)
		startOffset := offset + int64(lineStart)
		meta = append(meta, TextLineMetadata{
			Offset:       startOffset,
			Length:       len(lineBytes),
			RuneCount:    runes,
			DisplayWidth: width,
		})
		lines = append(lines, expanded)
		charCount += runes
		lineStart = lfIndex + 2
	}

	tail := data[lineStart:]
	if !truncated && len(tail) > 0 {
		if len(tail) >= 2 && isUTF16CR(tail[len(tail)-2], tail[len(tail)-1], encoding) {
			tail = tail[:len(tail)-2]
		}
		expanded, runes, width := expandUTF16Line(tail, encoding)
		startOffset := offset + int64(lineStart)
		meta = append(meta, TextLineMetadata{
			Offset:       startOffset,
			Length:       len(tail),
			RuneCount:    runes,
			DisplayWidth: width,
		})
		lines = append(lines, expanded)
		charCount += runes
		return lines, meta, charCount, nil
	}

	remainder := append([]byte(nil), tail...)
	return lines, meta, charCount, remainder
}

func isUTF16LF(lo, hi byte, enc fsutil.UnicodeEncoding) bool {
	if enc == fsutil.EncodingUTF16BE {
		return lo == 0x00 && hi == 0x0A
	}
	return lo == 0x0A && hi == 0x00
}

func isUTF16CR(lo, hi byte, enc fsutil.UnicodeEncoding) bool {
	if enc == fsutil.EncodingUTF16BE {
		return lo == 0x00 && hi == 0x0D
	}
	return lo == 0x0D && hi == 0x00
}

func expandUTF16Line(lineBytes []byte, enc fsutil.UnicodeEncoding) (string, int, int) {
	if len(lineBytes) == 0 {
		return "", 0, 0
	}
	endian := unicodeenc.LittleEndian
	if enc == fsutil.EncodingUTF16BE {
		endian = unicodeenc.BigEndian
	}
	decoder := unicodeenc.UTF16(endian, unicodeenc.IgnoreBOM).NewDecoder()
	utf8Bytes, err := decoder.Bytes(lineBytes)
	if err != nil {
		utf8Bytes = lineBytes
	}
	text := string(utf8Bytes)
	expanded := textutil.ExpandTabs(text, textutil.DefaultTabWidth)
	runes := utf8.RuneCountInString(expanded)
	width := textutil.DisplayWidth(expanded)
	return expanded, runes, width
}

func textLineMetadataFromLines(lines []string) []TextLineMetadata {
	if len(lines) == 0 {
		return nil
	}
	meta := make([]TextLineMetadata, len(lines))
	var offset int64
	for i, line := range lines {
		meta[i] = TextLineMetadata{
			Offset:       offset,
			Length:       len(line),
			RuneCount:    utf8.RuneCountInString(line),
			DisplayWidth: textutil.DisplayWidth(line),
		}
		offset += int64(len(line))
	}
	return meta
}

func textLineMetadataFromSegments(lines [][]StyledTextSegment) []TextLineMetadata {
	if len(lines) == 0 {
		return nil
	}
	meta := make([]TextLineMetadata, len(lines))
	var offset int64
	for i, line := range lines {
		width := 0
		length := 0
		for _, seg := range line {
			width += textutil.DisplayWidth(seg.Text)
			length += len(seg.Text)
		}
		meta[i] = TextLineMetadata{
			Offset:       offset,
			Length:       length,
			RuneCount:    utf8.RuneCountInString(joinSegmentsText(line)),
			DisplayWidth: width,
		}
		offset += int64(length)
	}
	return meta
}

// ===== REDUCER =====

// StateReducer applies actions to state
type StateReducer struct {
	selectionHistory   map[string]int // path -> selected index
	directoryCallbacks map[int][]directoryPostLoadFunc
}

// NewStateReducer creates a new reducer
func NewStateReducer() *StateReducer {
	return &StateReducer{
		selectionHistory:   make(map[string]int),
		directoryCallbacks: make(map[int][]directoryPostLoadFunc),
	}
}

func (r *StateReducer) enqueueDirectoryCallback(token int, fn directoryPostLoadFunc) {
	if token == 0 || fn == nil {
		return
	}
	r.directoryCallbacks[token] = append(r.directoryCallbacks[token], fn)
}

func (r *StateReducer) dropDirectoryCallbacks(token int) {
	if token == 0 {
		return
	}
	delete(r.directoryCallbacks, token)
}

func (r *StateReducer) runDirectoryCallbacks(state *AppState, token int) (bool, error) {
	callbacks, ok := r.directoryCallbacks[token]
	if !ok || len(callbacks) == 0 {
		return false, nil
	}
	delete(r.directoryCallbacks, token)

	for _, cb := range callbacks {
		if err := cb(r, state); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (r *StateReducer) completeDirectoryChange(state *AppState, loading bool, fn directoryPostLoadFunc) (*AppState, error) {
	if fn == nil {
		if loading {
			return state, nil
		}
		return state, r.generatePreview(state)
	}

	if loading {
		token := state.ActiveDirectoryLoadToken()
		r.enqueueDirectoryCallback(token, fn)
		return state, nil
	}

	if err := fn(r, state); err != nil {
		return state, err
	}
	return state, nil
}

func (r *StateReducer) changeDirectory(state *AppState, path string) error {
	_, err := r.changeDirectoryWithStatus(state, path)
	return err
}

func (r *StateReducer) changeDirectoryWithStatus(state *AppState, path string) (bool, error) {
	dirPath := path
	if dirPath == "" {
		dirPath = state.navigationPath()
	}
	dirPath = filepath.Clean(dirPath)

	loader := state.DirectoryLoader
	dispatch := state.getDispatch()
	if loader == nil || dispatch == nil {
		if err := LoadDirectory(state, dirPath); err != nil {
			return false, err
		}
		return false, nil
	}

	prevToken := state.ActiveDirectoryLoadToken()
	if prevToken != 0 {
		loader.Cancel(prevToken)
		r.dropDirectoryCallbacks(prevToken)
	}

	token := state.nextDirectoryLoadToken()
	state.setDirectoryLoadInFlight(token, dirPath)

	loader.Start(DirectoryLoadRequest{
		Token: token,
		Path:  dirPath,
		Callback: func(result DirectoryLoadResult) {
			dispatch(DirectoryLoadResultAction(result))
		},
	})

	return true, nil
}

func queryHasUppercase(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func updateCaseSensitivityOnAppend(current bool, ch rune) bool {
	if current {
		return true
	}
	return unicode.IsUpper(ch)
}

func isSearchWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func previousWordBoundary(runes []rune, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	i := pos - 1
	for i >= 0 && !isSearchWordChar(runes[i]) {
		i--
	}
	for i >= 0 && isSearchWordChar(runes[i]) {
		i--
	}
	return i + 1
}

func nextWordBoundary(runes []rune, pos int) int {
	if pos >= len(runes) {
		return len(runes)
	}
	if pos < 0 {
		pos = 0
	}

	i := pos
	for i < len(runes) && !isSearchWordChar(runes[i]) {
		i++
	}
	for i < len(runes) && isSearchWordChar(runes[i]) {
		i++
	}
	return i
}

func (r *StateReducer) triggerGlobalSearch(state *AppState) {
	state.GlobalSearchInProgress = true

	state.GlobalSearchID++
	searchID := state.GlobalSearchID

	dispatch := state.getDispatch()
	var progressFn func(IndexTelemetry)
	if dispatch != nil {
		progressFn = func(stats IndexTelemetry) {
			dispatch(GlobalSearchIndexProgressAction{Progress: stats})
		}
	}

	searcher := state.GlobalSearcher
	if searcher == nil || searcher.RootPath() != state.GlobalSearchRootPath || searcher.HideHidden() != state.HideHiddenFiles {
		if searcher != nil {
			searcher.CancelOngoingSearch()
		}
		searcher = searchpkg.NewGlobalSearcher(state.GlobalSearchRootPath, state.HideHiddenFiles, progressFn)
		state.GlobalSearcher = searcher
	}

	if searcher == nil {
		state.GlobalSearchStatus = SearchStatusIdle
		state.GlobalSearchInProgress = false
		return
	}

	state.GlobalSearchStatus = SearchStatusIndex

	state.GlobalSearchIndexStatus = searcher.CurrentProgress()

	query := state.CleanGlobalSearchQuery()
	caseSensitive := state.GlobalSearchCaseSensitive

	searcher.SearchRecursiveAsync(query, caseSensitive, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if state.GlobalSearchID != searchID {
			return
		}

		resultsCopy := make([]GlobalSearchResult, len(results))
		copy(resultsCopy, results)

		phase := SearchStatusIdle
		if inProgress {
			if isDone {
				phase = SearchStatusMerging
			} else {
				phase = SearchStatusIndex
			}
		}

		if dispatch != nil {
			dispatchPhase := phase
			if !inProgress {
				dispatchPhase = SearchStatusComplete
			}
			dispatch(GlobalSearchResultsAction{Results: resultsCopy, InProgress: inProgress, Phase: dispatchPhase})
			return
		}

		state.GlobalSearchResults = resultsCopy
		state.GlobalSearchInProgress = inProgress
		if phase != SearchStatusIdle {
			state.GlobalSearchStatus = phase
		} else if !inProgress {
			state.GlobalSearchStatus = SearchStatusComplete
		}
		state.clampGlobalSearchSelection()
	})
}

// Reduce applies an action to state and returns new state
// This is the PURE FUNCTION that determines all logic
func (r *StateReducer) Reduce(state *AppState, action Action) (*AppState, error) {
	// Make a shallow copy of state for immutability (or use pointers for efficiency)
	// In Go we'll mutate in place but conceptually treat it as immutable

	switch a := action.(type) {

	// ===== NAVIGATION =====

	case NavigateDownAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}

		displayIdx := state.getDisplaySelectedIndex()

		// If no selection yet (in filter mode with -1), start at 0
		if displayIdx < 0 {
			displayIdx = 0
		} else if displayIdx >= len(displayFiles)-1 {
			// Already at last item, nothing to do
			return state, nil
		} else {
			displayIdx++
		}

		state.setDisplaySelectedIndex(displayIdx)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case NavigateUpAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}

		displayIdx := state.getDisplaySelectedIndex()

		// If no selection yet (in filter mode with -1), start at last item
		if displayIdx < 0 {
			displayIdx = len(displayFiles) - 1
		} else {
			if displayIdx == 0 {
				// Already at first item
				return state, nil
			}
			displayIdx--
		}

		state.setDisplaySelectedIndex(displayIdx)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case EnterDirectoryAction:
		file := state.getCurrentFile()
		if file == nil || !file.IsDir {
			return state, nil
		}

		// Check if we're entering from a filtered view
		wasFilteredWhenEntering := state.FilterActive

		// Save current selection
		r.selectionHistory[state.CurrentPath] = state.SelectedIndex

		// Navigate to new directory
		newPath := filepath.Join(state.CurrentPath, file.Name)
		loading, err := r.changeDirectoryWithStatus(state, newPath)
		if err != nil {
			return state, err
		}

		post := func(r *StateReducer, state *AppState) error {
			// Clear global search when navigating to a new directory
			state.clearGlobalSearch(false)

			// Only restore saved selection if we DIDN'T enter from a filtered view
			// When entering from filter, start at first file (SelectedIndex = 0)
			if !wasFilteredWhenEntering {
				if savedIdx, ok := r.selectionHistory[newPath]; ok && savedIdx < len(state.Files) {
					state.SelectedIndex = savedIdx
					r.ensureSelectionVisible(state)
				}
			}

			// Center the selected file on screen when entering a directory
			state.centerScrollOnSelection()

			r.addToHistory(state, newPath)
			return r.generatePreview(state)
		}

		return r.completeDirectoryChange(state, loading, post)

	case GoUpAction:
		currentPath := state.navigationPath()
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			return state, nil // Already at root
		}

		// Save current selection
		r.selectionHistory[state.CurrentPath] = state.SelectedIndex

		// Find which directory we came from
		currentDirName := filepath.Base(currentPath)

		// Navigate to parent
		loading, err := r.changeDirectoryWithStatus(state, parent)
		if err != nil {
			return state, err
		}

		post := func(r *StateReducer, state *AppState) error {
			// Clear global search when navigating to a new directory
			state.clearGlobalSearch(false)

			// Find and select the directory we just came from
			for idx, f := range state.Files {
				if f.IsDir && f.Name == currentDirName {
					state.SelectedIndex = idx
					break
				}
			}

			r.ensureSelectionVisible(state)

			// Center the selected directory on screen
			state.centerScrollOnSelection()
			r.addToHistory(state, parent)
			return r.generatePreview(state)
		}

		return r.completeDirectoryChange(state, loading, post)

	case GoToPathAction:
		if a.Path == "" {
			return state, nil
		}
		target := filepath.Clean(a.Path)
		if target == state.navigationPath() {
			return state, r.generatePreview(state)
		}

		r.selectionHistory[state.CurrentPath] = state.SelectedIndex
		prevName := filepath.Base(state.CurrentPath)

		loading, err := r.changeDirectoryWithStatus(state, target)
		if err != nil {
			return state, err
		}

		post := func(r *StateReducer, state *AppState) error {
			state.clearGlobalSearch(false)

			// Prefer selecting the child we came from, if it exists.
			if prevName != "" {
				for idx, f := range state.Files {
					if f.Name == prevName {
						state.SelectedIndex = idx
						r.ensureSelectionVisible(state)
						state.centerScrollOnSelection()
						r.addToHistory(state, target)
						return r.generatePreview(state)
					}
				}
			}

			if savedIdx, ok := r.selectionHistory[target]; ok && savedIdx < len(state.Files) {
				state.SelectedIndex = savedIdx
				r.ensureSelectionVisible(state)
			} else {
				if len(state.Files) > 0 {
					state.SelectedIndex = 0
				} else {
					state.SelectedIndex = -1
				}
			}

			state.centerScrollOnSelection()
			r.addToHistory(state, target)
			return r.generatePreview(state)
		}

		return r.completeDirectoryChange(state, loading, post)

	case GoHomeAction:
		homeDir, err := userHomeDirFn()
		if err != nil {
			return state, fmt.Errorf("cannot resolve home directory: %w", err)
		}
		if homeDir == "" {
			return state, fmt.Errorf("home directory not available")
		}

		homeDir = filepath.Clean(homeDir)
		if homeDir == state.navigationPath() {
			return state, nil
		}

		r.selectionHistory[state.CurrentPath] = state.SelectedIndex

		loading, err := r.changeDirectoryWithStatus(state, homeDir)
		if err != nil {
			return state, err
		}

		post := func(r *StateReducer, state *AppState) error {
			state.clearGlobalSearch(false)

			if savedIdx, ok := r.selectionHistory[homeDir]; ok && savedIdx < len(state.Files) {
				state.SelectedIndex = savedIdx
				r.ensureSelectionVisible(state)
			}

			state.centerScrollOnSelection()
			r.addToHistory(state, homeDir)
			return r.generatePreview(state)
		}

		return r.completeDirectoryChange(state, loading, post)

	case GoToHistoryAction:
		switch a.Direction {
		case "back":
			if state.HistoryIndex > 0 {
				// Save current position before changing
				r.selectionHistory[state.CurrentPath] = state.SelectedIndex

				state.HistoryIndex--
				path := state.History[state.HistoryIndex]
				loading, err := r.changeDirectoryWithStatus(state, path)
				if err != nil {
					return state, err
				}

				post := func(r *StateReducer, state *AppState) error {
					// Clear global search when navigating to a new directory
					state.clearGlobalSearch(false)

					// Restore saved position AFTER changing directory
					// changeDirectory calls resetViewport which zeros SelectedIndex
					if savedIdx, ok := r.selectionHistory[path]; ok && savedIdx < len(state.Files) {
						state.SelectedIndex = savedIdx
						r.ensureSelectionVisible(state)
					}
					// Center the selected file on screen
					state.centerScrollOnSelection()
					return r.generatePreview(state)
				}

				return r.completeDirectoryChange(state, loading, post)
			}
		case "forward":
			if state.HistoryIndex < len(state.History)-1 {
				// Save current position before changing
				r.selectionHistory[state.CurrentPath] = state.SelectedIndex

				state.HistoryIndex++
				path := state.History[state.HistoryIndex]
				loading, err := r.changeDirectoryWithStatus(state, path)
				if err != nil {
					return state, err
				}

				post := func(r *StateReducer, state *AppState) error {
					// Clear global search when navigating to a new directory
					state.clearGlobalSearch(false)

					// Restore saved position AFTER changing directory
					// changeDirectory calls resetViewport which zeros SelectedIndex
					if savedIdx, ok := r.selectionHistory[path]; ok && savedIdx < len(state.Files) {
						state.SelectedIndex = savedIdx
						r.ensureSelectionVisible(state)
					}
					// Center the selected file on screen
					state.centerScrollOnSelection()
					return r.generatePreview(state)
				}

				return r.completeDirectoryChange(state, loading, post)
			}
		}
		return state, nil

	case RefreshDirectoryAction:
		snapshot := captureRefreshSnapshot(state)
		loading, err := r.changeDirectoryWithStatus(state, state.CurrentPath)
		if err != nil {
			return state, err
		}

		post := func(r *StateReducer, state *AppState) error {
			applyRefreshSnapshot(state, snapshot)
			return r.generatePreview(state)
		}

		return r.completeDirectoryChange(state, loading, post)

	case DirectoryLoadResultAction:
		if a.Token != state.ActiveDirectoryLoadToken() {
			return state, nil
		}

		state.clearDirectoryLoadingState()

		if a.Err != nil {
			state.LastError = a.Err
			r.dropDirectoryCallbacks(a.Token)
			return state, nil
		}

		applyDirectoryEntries(state, a.Path, a.Entries)

		ran, err := r.runDirectoryCallbacks(state, a.Token)
		if err != nil {
			return state, err
		}
		if !ran {
			return state, r.generatePreview(state)
		}
		return state, nil

	case PreviewLoadStartAction:
		pendingToken, pendingPath, pendingReset := state.previewPendingLoad()
		if pendingToken == 0 || pendingToken != a.Token || pendingPath == "" {
			return state, nil
		}

		state.cancelPreviewDebounceTimer()
		state.clearPreviewPendingLoad()

		loader := state.PreviewLoader
		dispatch := state.getDispatch()
		if loader == nil || dispatch == nil {
			preview, info, err := buildPreviewData(pendingPath, state.HideHiddenFiles)
			if err != nil {
				state.PreviewData = nil
				state.PreviewPath = ""
				state.resetPreviewScroll()
				return state, nil
			}
			state.clearPreviewLoadingState()
			r.applyPreviewToState(state, preview, info, pendingReset, pendingPath)
			return state, nil
		}

		// Cancel any inflight load (should already be cancelled, but be defensive).
		if state.ActivePreviewLoadToken() != 0 {
			loader.Cancel(state.ActivePreviewLoadToken())
		}

		state.setPreviewLoadInFlight(pendingToken, pendingPath, pendingReset)

		// If we have a fresh cache entry, show it immediately after debounce while
		// the loader refreshes in the background.
		if entry := state.getCurrentFile(); entry != nil {
			if info := fileInfoFromEntry(entry); info != nil {
				if cached, ok := state.getCachedFilePreview(pendingPath, info); ok {
					r.applyPreviewToState(state, cached, info, pendingReset, pendingPath)
				}
			}
		}

		loader.Start(PreviewLoadRequest{
			Token:      pendingToken,
			Path:       pendingPath,
			HideHidden: state.HideHiddenFiles,
			Callback: func(result PreviewLoadResult) {
				dispatch(PreviewLoadResultAction{
					Token:   result.Token,
					Path:    result.Path,
					Preview: result.Data,
					Info:    result.Info,
					Err:     result.Err,
				})
			},
		})
		return state, nil

	case PreviewLoadResultAction:
		if a.Token != state.ActivePreviewLoadToken() {
			return state, nil
		}

		resetScroll := state.previewShouldResetScroll()
		state.clearPreviewLoadingState()

		if a.Err != nil {
			state.LastError = a.Err
			state.PreviewData = nil
			state.PreviewPath = ""
			state.resetPreviewScroll()
			return state, nil
		}

		r.applyPreviewToState(state, a.Preview, a.Info, resetScroll, a.Path)
		return state, nil

	// ===== FILTERING =====

	case FilterStartAction:
		state.PreviewFullScreen = false
		wasActive := state.FilterActive
		if !wasActive {
			state.FilterSavedIndex = state.SelectedIndex // Save current selection
		}
		state.FilterActive = true
		state.FilterQuery = ""
		state.FilterCaseSensitive = false
		if wasActive {
			state.SelectedIndex = -1 // Reset when restarting filter
			state.ScrollOffset = 0
		}
		// Initialize FilteredIndices with all files (empty query shows all)
		// This ALWAYS resets FilteredIndices, even if filter was already active
		state.FilteredIndices = make([]int, len(state.Files))
		for i := range state.Files {
			state.FilteredIndices[i] = i
		}
		state.FilterMatches = nil // Clear old matches
		state.invalidateDisplayFilesCache()
		return state, nil

	case FilterCharAction:
		if state.FilterActive {
			prevSelectedIndex := state.SelectedIndex
			prevDisplayIdx := state.getDisplaySelectedIndex()
			prevTokenCount := countFilterTokens(state.FilterQuery)

			state.FilterQuery += string(a.Char)
			state.FilterCaseSensitive = updateCaseSensitivityOnAppend(state.FilterCaseSensitive, a.Char)
			if prevTokenCount == 0 && countFilterTokens(state.FilterQuery) > 0 {
				prevSelectedIndex = -1
				prevDisplayIdx = -1
			}
			state.recomputeFilter()
			state.retainSelectionAfterFilterChange(prevSelectedIndex, prevDisplayIdx)
			state.ScrollOffset = 0
			state.updateScrollVisibility()
		}
		return state, r.generatePreview(state)

	case FilterBackspaceAction:
		if state.FilterActive && len(state.FilterQuery) > 0 {
			prevSelectedIndex := state.SelectedIndex
			prevDisplayIdx := state.getDisplaySelectedIndex()

			runes := []rune(state.FilterQuery)
			runes = runes[:len(runes)-1]
			state.FilterQuery = string(runes)
			state.FilterCaseSensitive = queryHasUppercase(state.FilterQuery)
			if state.FilterQuery == "" {
				// When query becomes empty, stay in filter mode (don't clearFilter)
				// Show all files like FilterStartAction does
				state.FilteredIndices = make([]int, len(state.Files))
				for i := range state.Files {
					state.FilteredIndices[i] = i
				}
				state.FilterMatches = nil
				state.invalidateDisplayFilesCache()
				state.retainSelectionAfterFilterChange(prevSelectedIndex, prevDisplayIdx)
				state.FilterCaseSensitive = false
			} else {
				state.recomputeFilter()
				state.retainSelectionAfterFilterChange(prevSelectedIndex, prevDisplayIdx)
			}
			state.ScrollOffset = 0
			state.updateScrollVisibility()
		}
		return state, r.generatePreview(state)

	case FilterResetQueryAction:
		if state.FilterActive && len(state.FilterQuery) > 0 {
			prevSelectedIndex := state.SelectedIndex
			prevDisplayIdx := state.getDisplaySelectedIndex()

			state.FilterQuery = ""
			state.FilterCaseSensitive = false
			state.FilteredIndices = make([]int, len(state.Files))
			for i := range state.Files {
				state.FilteredIndices[i] = i
			}
			state.FilterMatches = nil
			state.invalidateDisplayFilesCache()
			state.retainSelectionAfterFilterChange(prevSelectedIndex, prevDisplayIdx)
			if state.SelectedIndex < 0 && len(state.Files) > 0 {
				state.SelectedIndex = 0
			}
			state.updateScrollVisibility()
		}
		return state, r.generatePreview(state)

	case FilterClearAction:
		// Only clear filter if filter is active
		if state.FilterActive {
			if state.SelectedIndex < 0 {
				if state.FilterSavedIndex >= 0 && state.FilterSavedIndex < len(state.Files) {
					state.SelectedIndex = state.FilterSavedIndex
				} else if len(state.Files) > 0 {
					state.SelectedIndex = 0
				}
			}
			state.clearFilter()
			// Keep the currently selected file from the filtered results
			// (don't restore the selection from before entering filter mode)
			// Center the view when exiting filter mode - this is a contextual navigation
			state.centerScrollOnSelection()
			return state, r.generatePreview(state)
		}
		// If filter is not active, do nothing (don't reset cursor)
		return state, nil

	// ===== SCROLLING =====

	case ScrollUpAction:
		displayIdx := state.getDisplaySelectedIndex()
		if displayIdx <= 0 {
			return state, nil
		}

		state.setDisplaySelectedIndex(displayIdx - 1)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case ScrollDownAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}

		displayIdx := state.getDisplaySelectedIndex()
		if displayIdx >= len(displayFiles)-1 {
			return state, nil
		}

		state.setDisplaySelectedIndex(displayIdx + 1)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case ScrollPageUpAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}

		visibleLines := state.visibleLines()
		if visibleLines <= 0 {
			return state, nil
		}

		displayIdx := state.getDisplaySelectedIndex()

		newIdx := displayIdx - visibleLines
		if newIdx < 0 {
			newIdx = 0
		}
		if newIdx == displayIdx {
			return state, nil
		}
		state.setDisplaySelectedIndex(newIdx)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case ScrollPageDownAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}

		visibleLines := state.visibleLines()
		if visibleLines <= 0 {
			return state, nil
		}

		displayIdx := state.getDisplaySelectedIndex()
		newIdx := displayIdx + visibleLines
		if newIdx >= len(displayFiles) {
			newIdx = len(displayFiles) - 1
		}
		if newIdx == displayIdx {
			return state, nil
		}
		state.setDisplaySelectedIndex(newIdx)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case ScrollToStartAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}
		if state.getDisplaySelectedIndex() == 0 {
			return state, nil
		}
		state.setDisplaySelectedIndex(0)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case ScrollToEndAction:
		displayFiles := state.getDisplayFiles()
		if len(displayFiles) == 0 {
			return state, nil
		}
		if state.getDisplaySelectedIndex() == len(displayFiles)-1 {
			return state, nil
		}
		state.setDisplaySelectedIndex(len(displayFiles) - 1)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	case MouseSelectAction:
		displayFiles := state.getDisplayFiles()
		if a.DisplayIndex < 0 || a.DisplayIndex >= len(displayFiles) {
			return state, nil
		}
		state.setDisplaySelectedIndex(a.DisplayIndex)
		state.updateScrollVisibility()
		return state, r.generatePreview(state)

	// ===== VIEW =====

	case ResizeAction:
		state.ScreenWidth = a.Width
		state.ScreenHeight = a.Height
		state.updateScrollVisibility()
		state.clampPreviewScroll()
		return state, nil

		// ===== PREVIEW =====

	case PreviewEnterFullScreenAction:
		if state.PreviewData == nil {
			if err := r.generatePreview(state); err != nil {
				return state, err
			}
		}
		if state.PreviewData != nil {
			state.PreviewFullScreen = true
			restored := state.restorePreviewScrollForPath(state.CurrentFilePath())
			if !restored {
				state.normalizePreviewScroll()
			}
			if state.PreviewData.TextTruncated {
				if state.PreviewScrollOffset < 0 {
					state.PreviewScrollOffset = 0
				}
				if state.PreviewWrapOffset < 0 {
					state.PreviewWrapOffset = 0
				}
				// Don't clamp downward for truncated previews; the pager will stream
				// on demand to satisfy the remembered offset.
			} else {
				state.clampPreviewScroll()
			}
		}
		return state, nil

	case PreviewExitFullScreenAction:
		if state.PreviewFullScreen {
			state.rememberPreviewScrollForCurrentFile()
			state.PreviewFullScreen = false
			// Reset inline preview to the top; the remembered position is kept
			// in history and will be restored next time fullscreen pager opens.
			state.normalizePreviewScroll()
			state.clampPreviewScroll()
		}
		return state, nil

	case PreviewScrollUpAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			state.scrollPreviewBy(-1)
		}
		return state, nil

	case PreviewScrollDownAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			state.scrollPreviewBy(1)
		}
		return state, nil

	case PreviewScrollPageUpAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			lines := state.previewVisibleLines()
			if lines <= 0 {
				lines = 1
			}
			state.scrollPreviewBy(-lines)
		}
		return state, nil

	case PreviewScrollPageDownAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			lines := state.previewVisibleLines()
			if lines <= 0 {
				lines = 1
			}
			state.scrollPreviewBy(lines)
		}
		return state, nil

	case PreviewScrollToStartAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			state.PreviewScrollOffset = 0
			state.PreviewWrapOffset = 0
		}
		return state, nil

	case PreviewScrollToEndAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			state.PreviewScrollOffset = state.maxPreviewScrollOffset()
			state.PreviewWrapOffset = 0
		}
		return state, nil

	case TogglePreviewWrapAction:
		if state.PreviewFullScreen && state.PreviewData != nil {
			state.PreviewWrap = !state.PreviewWrap
			if !state.PreviewWrap {
				state.PreviewWrapOffset = 0
			}
		}
		return state, nil

	// ===== GLOBAL SEARCH =====

	case GlobalSearchStartAction:
		state.PreviewFullScreen = false
		// Start global search from current directory
		state.GlobalSearchActive = true
		state.setGlobalSearchQuery("")
		state.GlobalSearchCursorPos = 0
		state.GlobalSearchCaseSensitive = false
		state.GlobalSearchResults = nil
		state.GlobalSearchIndex = 0
		state.GlobalSearchScroll = 0
		state.GlobalSearchDesiredSelectionPath = ""
		state.clearGlobalSearchPendingIndex()
		if state.LastGlobalSearchQuery != "" && state.LastGlobalSearchRootPath == state.CurrentPath {
			state.setGlobalSearchQuery(state.LastGlobalSearchQuery)
			state.GlobalSearchCursorPos = len([]rune(state.GlobalSearchQuery))
			state.GlobalSearchCaseSensitive = queryHasUppercase(state.GlobalSearchQuery)
			state.GlobalSearchIndex = state.LastGlobalSearchIndex
			if state.GlobalSearchIndex < 0 {
				state.GlobalSearchIndex = 0
			}
			state.GlobalSearchScroll = state.LastGlobalSearchScroll
			if state.GlobalSearchScroll < 0 {
				state.GlobalSearchScroll = 0
			}
			state.GlobalSearchDesiredSelectionPath = state.LastGlobalSearchSelectionPath
			state.setGlobalSearchPendingIndex(state.LastGlobalSearchIndex)
		}
		state.GlobalSearchRootPath = state.CurrentPath
		state.GlobalSearchIndexStatus = IndexTelemetry{}

		if state.GlobalSearcher != nil {
			state.GlobalSearcher.CancelOngoingSearch()
		}
		state.GlobalSearcher = nil

		r.triggerGlobalSearch(state)
		return state, nil

	case GlobalSearchCharAction:
		if state.GlobalSearchActive {
			prevResults := state.GlobalSearchResults
			prevQuery := state.CleanGlobalSearchQuery()
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			runes := []rune(state.GlobalSearchQuery)
			cursor := state.GlobalSearchCursorPos
			if cursor < 0 {
				cursor = 0
			}
			if cursor > len(runes) {
				cursor = len(runes)
			}

			var buffer []rune
			buffer = append(buffer, runes[:cursor]...)
			buffer = append(buffer, a.Char)
			buffer = append(buffer, runes[cursor:]...)

			state.setGlobalSearchQuery(string(buffer))
			state.GlobalSearchCursorPos = cursor + 1
			state.GlobalSearchCaseSensitive = queryHasUppercase(state.GlobalSearchQuery)

			if state.CleanGlobalSearchQuery() == "" {
				state.GlobalSearchCaseSensitive = false
			}

			r.applyLocalSearchPreview(state, prevResults, prevQuery)
			r.triggerGlobalSearch(state)
		}
		return state, nil

	case GlobalSearchBackspaceAction:
		if state.GlobalSearchActive && len(state.CleanGlobalSearchQuery()) > 0 {
			prevResults := state.GlobalSearchResults
			prevQuery := state.CleanGlobalSearchQuery()
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			runes := []rune(state.GlobalSearchQuery)
			cursor := state.GlobalSearchCursorPos
			if cursor < 0 {
				cursor = 0
			}
			if cursor > len(runes) {
				cursor = len(runes)
			}

			if len(runes) == 0 || cursor == 0 {
				return state, nil
			}

			buffer := append([]rune{}, runes[:cursor-1]...)
			buffer = append(buffer, runes[cursor:]...)

			state.setGlobalSearchQuery(string(buffer))
			newCursor := cursor - 1
			if newCursor < 0 {
				newCursor = 0
			}
			state.GlobalSearchCursorPos = newCursor
			state.GlobalSearchCaseSensitive = queryHasUppercase(state.GlobalSearchQuery)
			if state.CleanGlobalSearchQuery() == "" {
				state.GlobalSearchCaseSensitive = false
			}

			r.applyLocalSearchPreview(state, prevResults, prevQuery)
			r.triggerGlobalSearch(state)
		}
		return state, nil

	case GlobalSearchDeleteAction:
		if state.GlobalSearchActive && len(state.CleanGlobalSearchQuery()) > 0 {
			prevResults := state.GlobalSearchResults
			prevQuery := state.CleanGlobalSearchQuery()
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			runes := []rune(state.GlobalSearchQuery)
			cursor := state.GlobalSearchCursorPos
			if cursor < 0 {
				cursor = 0
			}
			if cursor >= len(runes) {
				return state, nil
			}

			buffer := append([]rune{}, runes[:cursor]...)
			buffer = append(buffer, runes[cursor+1:]...)

			state.setGlobalSearchQuery(string(buffer))
			state.GlobalSearchCaseSensitive = queryHasUppercase(state.GlobalSearchQuery)
			if state.CleanGlobalSearchQuery() == "" {
				state.GlobalSearchCaseSensitive = false
			}

			r.applyLocalSearchPreview(state, prevResults, prevQuery)
			r.triggerGlobalSearch(state)
		}
		return state, nil

	case GlobalSearchDeleteWordAction:
		if state.GlobalSearchActive && len(state.CleanGlobalSearchQuery()) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			runes := []rune(state.GlobalSearchQuery)
			cursor := state.GlobalSearchCursorPos
			if cursor < 0 {
				cursor = 0
			}
			if cursor > len(runes) {
				cursor = len(runes)
			}
			if cursor == 0 {
				return state, nil
			}

			start := previousWordBoundary(runes, cursor)
			if start < 0 {
				start = 0
			}

			buffer := append([]rune{}, runes[:start]...)
			buffer = append(buffer, runes[cursor:]...)

			state.setGlobalSearchQuery(string(buffer))
			state.GlobalSearchCursorPos = start
			state.GlobalSearchCaseSensitive = queryHasUppercase(state.GlobalSearchQuery)
			if state.CleanGlobalSearchQuery() == "" {
				state.GlobalSearchCaseSensitive = false
			}

			r.triggerGlobalSearch(state)
		}
		return state, nil

	case GlobalSearchMoveCursorAction:
		if state.GlobalSearchActive {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			runes := []rune(state.GlobalSearchQuery)
			switch a.Direction {
			case "left":
				if state.GlobalSearchCursorPos > 0 {
					state.GlobalSearchCursorPos--
				}
			case "right":
				if state.GlobalSearchCursorPos < len(runes) {
					state.GlobalSearchCursorPos++
				}
			case "word-left":
				state.GlobalSearchCursorPos = previousWordBoundary(runes, state.GlobalSearchCursorPos)
			case "word-right":
				state.GlobalSearchCursorPos = nextWordBoundary(runes, state.GlobalSearchCursorPos)
			case "home":
				state.GlobalSearchCursorPos = 0
			case "end":
				state.GlobalSearchCursorPos = len(runes)
			}
		}
		return state, nil

	case GlobalSearchResetQueryAction:
		if state.GlobalSearchActive && state.CleanGlobalSearchQuery() != "" {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			state.setGlobalSearchQuery("")
			state.GlobalSearchCursorPos = 0
			state.GlobalSearchCaseSensitive = false
			state.GlobalSearchIndex = 0
			state.GlobalSearchResults = nil
			r.triggerGlobalSearch(state)
		}
		return state, nil

	case GlobalSearchClearAction:
		state.clearGlobalSearch(true)
		return state, r.generatePreview(state)

	case GlobalSearchIndexProgressAction:
		progress := a.Progress
		if progress.RootPath == "" {
			return state, nil
		}
		if state.GlobalSearcher != nil && state.GlobalSearcher.RootPath() == progress.RootPath {
			state.GlobalSearchIndexStatus = progress
		} else if state.GlobalSearchRootPath == progress.RootPath {
			state.GlobalSearchIndexStatus = progress
		}

		return state, nil

	case GlobalSearchResultsAction:
		prevResults := state.GlobalSearchResults
		prevIndex := state.GlobalSearchIndex

		state.GlobalSearchResults = make([]GlobalSearchResult, len(a.Results))
		copy(state.GlobalSearchResults, a.Results)
		state.GlobalSearchInProgress = a.InProgress
		if a.Phase != SearchStatusIdle {
			state.GlobalSearchStatus = a.Phase
		}
		if !a.InProgress {
			state.GlobalSearchStatus = SearchStatusComplete
		}
		state.restoreGlobalSearchSelection(prevResults, prevIndex)
		state.clampGlobalSearchSelection()
		state.applyDesiredGlobalSearchSelection()
		state.applyPendingGlobalSearchIndex()
		return state, nil

	case GlobalSearchNavigateAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			if a.Direction == "up" && state.GlobalSearchIndex > 0 {
				state.GlobalSearchIndex--
				state.updateGlobalSearchScroll()
			} else if a.Direction == "down" && state.GlobalSearchIndex < len(state.GlobalSearchResults)-1 {
				state.GlobalSearchIndex++
				state.updateGlobalSearchScroll()
			}
		}
		return state, nil

	case GlobalSearchSelectIndexAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			idx := a.Index
			if idx < 0 {
				idx = 0
			}
			if idx >= len(state.GlobalSearchResults) {
				idx = len(state.GlobalSearchResults) - 1
			}
			state.GlobalSearchIndex = idx
			state.updateGlobalSearchScroll()
		}
		return state, nil

	case GlobalSearchPageUpAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			// Jump up by viewport height
			pageSize := state.visibleLines() // Account for status bar and borders
			newIdx := state.GlobalSearchIndex - pageSize
			if newIdx < 0 {
				newIdx = 0
			}
			state.GlobalSearchIndex = newIdx
			state.updateGlobalSearchScroll()
		}
		return state, nil

	case GlobalSearchPageDownAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			// Jump down by viewport height
			pageSize := state.visibleLines() // Account for status bar and borders
			newIdx := state.GlobalSearchIndex + pageSize
			maxIdx := len(state.GlobalSearchResults) - 1
			if newIdx > maxIdx {
				newIdx = maxIdx
			}
			state.GlobalSearchIndex = newIdx
			state.updateGlobalSearchScroll()
		}
		return state, nil

	case GlobalSearchHomeAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			state.GlobalSearchIndex = 0
			state.updateGlobalSearchScroll()
		}
		return state, nil

	case GlobalSearchEndAction:
		if state.GlobalSearchActive && len(state.GlobalSearchResults) > 0 {
			state.clearDesiredGlobalSearchSelection()
			state.clearGlobalSearchPendingIndex()
			state.GlobalSearchIndex = len(state.GlobalSearchResults) - 1
			state.updateGlobalSearchScroll()
		}
		return state, nil

	case GlobalSearchOpenAction:
		if state.GlobalSearchActive && state.GlobalSearchIndex >= 0 && state.GlobalSearchIndex < len(state.GlobalSearchResults) {
			result := state.GlobalSearchResults[state.GlobalSearchIndex]

			// Save current selection before navigating
			r.selectionHistory[state.CurrentPath] = state.SelectedIndex

			// Navigate to the directory containing the file
			loading, err := r.changeDirectoryWithStatus(state, result.DirPath)
			if err != nil {
				return state, err
			}

			post := func(r *StateReducer, state *AppState) error {
				// Find and select the file in the new directory
				for i, f := range state.Files {
					if f.Name == result.FileName {
						state.SelectedIndex = i
						break
					}
				}

				state.updateScrollVisibility()

				// Add to history just like normal navigation
				r.addToHistory(state, result.DirPath)

				// Close global search after navigating
				state.clearGlobalSearch(false)
				return r.generatePreview(state)
			}

			return r.completeDirectoryChange(state, loading, post)
		}
		return state, nil

	case ToggleHiddenFilesAction:
		// IMPORTANT: Remember display position BEFORE toggle
		// This is needed for fuzzy search, which may reorder files
		originalIdx := state.SelectedIndex
		var displayIdxBeforeToggle int
		if state.FilterActive {
			// When filter is active, remember position in display order (FilteredIndices)
			// This is used later to search for nearest visible in display order
			displayIdxBeforeToggle = -1
			for i, idx := range state.FilteredIndices {
				if idx == originalIdx {
					displayIdxBeforeToggle = i
					break
				}
			}
		}

		// Toggle and recompute filter
		state.HideHiddenFiles = !state.HideHiddenFiles
		state.recomputeFilter()
		state.updateParentEntries()

		// Adjust cursor position based on new visibility
		displayFiles := state.getDisplayFiles()

		if len(displayFiles) == 0 {
			// No visible files left, reset selection
			state.SelectedIndex = -1
		} else if originalIdx < 0 {
			// No previous selection, select first file in display
			// Find the first file that's in the display
			if state.FilterActive && len(state.FilteredIndices) > 0 {
				state.SelectedIndex = state.FilteredIndices[0]
			} else {
				// Find first non-hidden file (respecting current HideHiddenFiles setting)
				for i, f := range state.Files {
					if !f.IsHidden() || !state.HideHiddenFiles {
						state.SelectedIndex = i
						break
					}
				}
			}
		} else if originalIdx >= 0 && originalIdx < len(state.Files) {
			// Check if currently selected file is still valid with new visibility settings
			currentFile := state.Files[originalIdx]

			// First check: is the current file hidden?
			if state.HideHiddenFiles && currentFile.IsHidden() {
				// Current selection is now hidden, find nearest visible file
				found := false

				if state.FilterActive {
					// When filter is active, search in DISPLAY ORDER (via FilteredIndices)
					// This respects fuzzy search order instead of Files array order
					// Use displayIdxBeforeToggle which was captured before toggle
					displayIdx := displayIdxBeforeToggle

					if displayIdx >= 0 && displayIdx < len(state.FilteredIndices) {
						// Search backward in FilteredIndices
						for i := displayIdx - 1; i >= 0; i-- {
							fileIdx := state.FilteredIndices[i]
							if !state.Files[fileIdx].IsHidden() {
								state.SelectedIndex = fileIdx
								found = true
								break
							}
						}

						// Search forward in FilteredIndices
						if !found {
							for i := displayIdx + 1; i < len(state.FilteredIndices); i++ {
								fileIdx := state.FilteredIndices[i]
								if !state.Files[fileIdx].IsHidden() {
									state.SelectedIndex = fileIdx
									found = true
									break
								}
							}
						}
					}

					// Fallback: select first visible in FilteredIndices
					if !found && len(state.FilteredIndices) > 0 {
						for _, idx := range state.FilteredIndices {
							if !state.Files[idx].IsHidden() {
								state.SelectedIndex = idx
								break
							}
						}
					}
				} else {
					// When filter is NOT active, search all files
					for i := originalIdx - 1; i >= 0; i-- {
						if !state.Files[i].IsHidden() {
							state.SelectedIndex = i
							found = true
							break
						}
					}
					if !found {
						for i := originalIdx + 1; i < len(state.Files); i++ {
							if !state.Files[i].IsHidden() {
								state.SelectedIndex = i
								found = true
								break
							}
						}
					}
					if !found {
						for i, f := range state.Files {
							if !f.IsHidden() {
								state.SelectedIndex = i
								break
							}
						}
					}
				}
			} else if state.FilterActive {
				// Second check: if filter is active, verify selected file is still visible
				// Note: FilteredIndices contains file indices, but getDisplayFiles() applies HideHiddenFiles
				// so we need to verify the file is actually visible in getDisplayFiles()
				stillVisible := false
				for _, f := range displayFiles {
					if f.Name == currentFile.Name {
						stillVisible = true
						break
					}
				}

				if !stillVisible {
					// Current file is not visible in display (either not in filter or now hidden)
					// Find nearest file in FilteredIndices that is non-hidden (or visible)
					found := false

					// Try to find a file above the current position
					for i := originalIdx - 1; i >= 0; i-- {
						inFiltered := false
						for _, idx := range state.FilteredIndices {
							if idx == i {
								inFiltered = true
								break
							}
						}
						if inFiltered && (!state.HideHiddenFiles || !state.Files[i].IsHidden()) {
							state.SelectedIndex = i
							found = true
							break
						}
					}

					// If not found above, try below
					if !found {
						for i := originalIdx + 1; i < len(state.Files); i++ {
							inFiltered := false
							for _, idx := range state.FilteredIndices {
								if idx == i {
									inFiltered = true
									break
								}
							}
							if inFiltered && (!state.HideHiddenFiles || !state.Files[i].IsHidden()) {
								state.SelectedIndex = i
								found = true
								break
							}
						}
					}

					// If still not found, select first visible in filtered results
					if !found && len(state.FilteredIndices) > 0 {
						for _, idx := range state.FilteredIndices {
							if !state.HideHiddenFiles || !state.Files[idx].IsHidden() {
								state.SelectedIndex = idx
								break
							}
						}
					}
				}
			}
			// If current file is not hidden and (filter is inactive or it's in filtered results), keep SelectedIndex as is
		}

		// Center the selected file on screen when toggling hidden files visibility
		// This prevents cursor from jumping to bottom when many hidden files appear/disappear
		state.centerScrollOnSelection()
		return state, r.generatePreview(state)

	case HelpToggleAction:
		state.HelpVisible = !state.HelpVisible
		return state, nil

	case HelpHideAction:
		if state.HelpVisible {
			state.HelpVisible = false
		}
		return state, nil

	default:
		return state, fmt.Errorf("unknown action: %T", action)
	}
}

func (r *StateReducer) applyLocalSearchPreview(state *AppState, prevResults []GlobalSearchResult, prevQuery string) {
	query := state.CleanGlobalSearchQuery()
	if query == "" {
		return
	}

	if state.GlobalSearcher != nil {
		if cached, ok := state.GlobalSearcher.CachedResults(query, state.GlobalSearchCaseSensitive); ok && len(cached) > 0 {
			state.GlobalSearchResults = cloneResults(cached)
			state.GlobalSearchInProgress = true
			state.GlobalSearchStatus = SearchStatusIndex
			state.clampGlobalSearchSelection()
			return
		}
	}

	if prevQuery == "" || len(prevResults) == 0 {
		return
	}

	if !isQueryExtension(prevQuery, query, state.GlobalSearchCaseSensitive) {
		return
	}

	filtered := filterResultsByQuery(prevResults, query, state.GlobalSearchCaseSensitive)
	if len(filtered) == 0 {
		return
	}
	state.GlobalSearchResults = filtered
	state.GlobalSearchInProgress = true
	state.GlobalSearchStatus = SearchStatusIndex
	state.clampGlobalSearchSelection()
}

func cloneResults(results []GlobalSearchResult) []GlobalSearchResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]GlobalSearchResult, len(results))
	copy(out, results)
	return out
}

func isQueryExtension(prev, current string, caseSensitive bool) bool {
	if prev == "" {
		return false
	}
	if !caseSensitive {
		prev = strings.ToLower(prev)
		current = strings.ToLower(current)
	}
	return strings.HasPrefix(current, prev)
}

func filterResultsByQuery(results []GlobalSearchResult, query string, caseSensitive bool) []GlobalSearchResult {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return nil
	}
	if !caseSensitive {
		for i := range tokens {
			tokens[i] = strings.ToLower(tokens[i])
		}
	}

	filtered := make([]GlobalSearchResult, 0, len(results))
	for _, res := range results {
		path := res.FilePath
		compare := path
		if !caseSensitive {
			compare = strings.ToLower(compare)
		}
		match := true
		for _, tok := range tokens {
			if !strings.Contains(compare, tok) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, res)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	out := make([]GlobalSearchResult, len(filtered))
	copy(out, filtered)
	return out
}

// ===== PRIVATE HELPER METHODS =====

func (r *StateReducer) ensureSelectionVisible(state *AppState) {
	if !state.HideHiddenFiles {
		return
	}
	if state.SelectedIndex < 0 || state.SelectedIndex >= len(state.Files) {
		return
	}
	if !state.Files[state.SelectedIndex].IsHidden() {
		return
	}

	// Prefer the closest visible file above the current selection
	for i := state.SelectedIndex - 1; i >= 0; i-- {
		if !state.Files[i].IsHidden() {
			state.SelectedIndex = i
			return
		}
	}

	// If none above, search forward
	for i := state.SelectedIndex + 1; i < len(state.Files); i++ {
		if !state.Files[i].IsHidden() {
			state.SelectedIndex = i
			return
		}
	}

	// No visible files remain
	state.SelectedIndex = -1
}

type filterSnapshot struct {
	active        bool
	query         string
	caseSensitive bool
	savedIndex    int
}

func snapshotFilterState(state *AppState) filterSnapshot {
	return filterSnapshot{
		active:        state.FilterActive,
		query:         state.FilterQuery,
		caseSensitive: state.FilterCaseSensitive,
		savedIndex:    state.FilterSavedIndex,
	}
}

func findFileIndexByName(files []FileEntry, name string) int {
	for idx, file := range files {
		if file.Name == name {
			return idx
		}
	}
	return -1
}

type refreshSnapshot struct {
	prevFileName      string
	prevSelectedIndex int
	prevDisplayIdx    int
	prevScrollOffset  int
	prevFilter        filterSnapshot
}

func captureRefreshSnapshot(state *AppState) refreshSnapshot {
	prevFile := state.getCurrentFile()
	prevFileName := ""
	if prevFile != nil {
		prevFileName = prevFile.Name
	}

	return refreshSnapshot{
		prevFileName:      prevFileName,
		prevSelectedIndex: state.SelectedIndex,
		prevDisplayIdx:    state.getDisplaySelectedIndex(),
		prevScrollOffset:  state.ScrollOffset,
		prevFilter:        snapshotFilterState(state),
	}
}

func applyRefreshSnapshot(state *AppState, snap refreshSnapshot) {
	restoredIndex := -1
	if snap.prevFileName != "" {
		if idx := findFileIndexByName(state.Files, snap.prevFileName); idx >= 0 {
			state.SelectedIndex = idx
			restoredIndex = idx
		}
	}

	if restoredIndex == -1 && snap.prevSelectedIndex >= 0 {
		if snap.prevSelectedIndex < len(state.Files) {
			state.SelectedIndex = snap.prevSelectedIndex
		} else if len(state.Files) > 0 {
			state.SelectedIndex = len(state.Files) - 1
		} else {
			state.SelectedIndex = -1
		}
		restoredIndex = state.SelectedIndex
	}

	if snap.prevFilter.active {
		state.FilterActive = true
		state.FilterQuery = snap.prevFilter.query
		state.FilterCaseSensitive = snap.prevFilter.caseSensitive
		state.FilterSavedIndex = snap.prevFilter.savedIndex
		state.recomputeFilter()
		state.retainSelectionAfterFilterChange(restoredIndex, snap.prevDisplayIdx)
	}

	state.ScrollOffset = snap.prevScrollOffset
	state.updateScrollVisibility()
}

func (r *StateReducer) cancelPreviewLoad(state *AppState) {
	if state == nil {
		return
	}
	token := state.ActivePreviewLoadToken()
	if token != 0 && state.PreviewLoader != nil {
		state.PreviewLoader.Cancel(token)
	}
	state.clearPreviewLoadingState()
	state.cancelPreviewDebounceTimer()
	state.clearPreviewPendingLoad()
}

func (r *StateReducer) applyPreviewToState(state *AppState, preview *PreviewData, info os.FileInfo, resetScroll bool, path string) {
	if state == nil {
		return
	}
	if preview == nil {
		state.PreviewData = nil
		state.PreviewPath = ""
		state.resetPreviewScroll()
		return
	}

	if info != nil {
		state.storeFilePreview(path, info, preview)
	}

	state.PreviewData = preview
	state.PreviewPath = path
	if resetScroll {
		if path != "" && state.restorePreviewScrollForPath(path) {
			state.clampPreviewScroll()
		} else {
			state.PreviewScrollOffset = 0
			state.PreviewWrapOffset = 0
		}
		state.PreviewFullScreen = false
	} else {
		state.clampPreviewScroll()
		state.rememberPreviewScrollForCurrentFile()
	}
}

// addToHistory adds path to history, removing forward history if needed
func (r *StateReducer) addToHistory(state *AppState, path string) {
	// If target matches previous entry, just move back in history (no mutation).
	if state.HistoryIndex > 0 && state.History[state.HistoryIndex-1] == path {
		state.HistoryIndex--
		return
	}

	// If target matches next entry, move forward in history.
	if state.HistoryIndex < len(state.History)-1 && state.History[state.HistoryIndex+1] == path {
		state.HistoryIndex++
		return
	}

	// If not at end of history and target is a new branch, truncate forward.
	if state.HistoryIndex < len(state.History)-1 {
		state.History = state.History[:state.HistoryIndex+1]
	}

	// Add new path if different from current
	if len(state.History) == 0 || state.History[len(state.History)-1] != path {
		state.History = append(state.History, path)
		state.HistoryIndex = len(state.History) - 1
	}
}

// generatePreview creates preview data for selected file
func (r *StateReducer) generatePreview(state *AppState) error {
	file := state.getCurrentFile()
	if file == nil {
		r.cancelPreviewLoad(state)
		state.PreviewData = nil
		state.resetPreviewScroll()
		return nil
	}

	filePath := filepath.Join(state.CurrentPath, file.Name)
	sameFile := state.PreviewData != nil &&
		state.PreviewData.Name == file.Name &&
		state.PreviewData.IsDir == file.IsDir
	resetScroll := !sameFile

	loader := state.PreviewLoader
	dispatch := state.getDispatch()
	if loader == nil || dispatch == nil {
		// Synchronous path: reuse cache immediately if it matches before doing any work.
		if info := fileInfoFromEntry(file); info != nil {
			if cached, ok := state.getCachedFilePreview(filePath, info); ok {
				r.applyPreviewToState(state, cached, info, resetScroll, filePath)
				return nil
			}
		}

		r.cancelPreviewLoad(state)
		preview, info, err := buildPreviewData(filePath, state.HideHiddenFiles)
		if err != nil {
			state.PreviewData = nil
			state.PreviewPath = ""
			state.resetPreviewScroll()
			return nil
		}
		state.clearPreviewLoadingState()
		r.applyPreviewToState(state, preview, info, resetScroll, filePath)
		return nil
	}

	// If a load is currently in progress for another file, cancel it.
	if state.PreviewLoading && state.PreviewLoadingPath != "" && state.PreviewLoadingPath != filePath {
		r.cancelPreviewLoad(state)
	}

	// If a load is already pending for this path, just update reset flag.
	if pendingToken, pendingPath, _ := state.previewPendingLoad(); pendingPath == filePath && pendingToken != 0 {
		state.setPreviewPendingLoad(pendingToken, filePath, resetScroll)
		return nil
	} else if pendingPath != "" && pendingPath != filePath {
		state.cancelPreviewDebounceTimer()
		state.clearPreviewPendingLoad()
	}

	// If a load is currently running for this same path, keep waiting.
	if state.PreviewLoading && state.PreviewLoadingPath == filePath {
		state.pendingPreviewReset = resetScroll
		return nil
	}

	state.cancelPreviewDebounceTimer()

	token := state.nextPreviewLoadToken()
	state.setPreviewPendingLoad(token, filePath, resetScroll)
	state.previewDebounceTimer = time.AfterFunc(previewDebounceDelay, func() {
		dispatch(PreviewLoadStartAction{Token: token})
	})
	return nil
}

// GeneratePreview exposes the preview-building helper to other packages (e.g., initial boot).
func (r *StateReducer) GeneratePreview(state *AppState) error {
	return r.generatePreview(state)
}

// EnsurePreviewCurrent forces the preview to match the currently selected file,
// bypassing debounce/async gaps. Useful when the user opens the fullscreen
// pager immediately after moving the cursor.
func (r *StateReducer) EnsurePreviewCurrent(state *AppState) error {
	if state == nil {
		return nil
	}
	filePath := state.CurrentFilePath()
	if filePath == "" {
		return nil
	}

	if state.PreviewData != nil && state.PreviewPath == filePath && !state.PreviewLoading {
		return nil
	}

	// Cancel any in-flight preview for another file.
	if state.PreviewLoading && state.PreviewLoadingPath != "" && state.PreviewLoadingPath != filePath {
		r.cancelPreviewLoad(state)
	}
	state.cancelPreviewDebounceTimer()
	state.clearPreviewPendingLoad()

	preview, info, err := buildPreviewData(filePath, state.HideHiddenFiles)
	if err != nil {
		state.PreviewData = nil
		state.resetPreviewScroll()
		state.LastError = err
		return err
	}

	state.clearPreviewLoadingState()
	r.applyPreviewToState(state, preview, info, true, filePath)
	return nil
}
