package state

import (
	"os"
	"strings"
	"time"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	search "github.com/kk-code-lab/rdir/internal/search"
)

// FileEntry mirrors fs.Entry so UI/state code can rely on a stable type.
type FileEntry = fsutil.Entry
type FuzzyMatch = search.FuzzyMatch
type FuzzyMatcher = search.FuzzyMatcher
type MatchDetails = search.MatchDetails
type MatchSpan = search.MatchSpan
type GlobalSearchResult = search.GlobalSearchResult
type GlobalSearcher = search.GlobalSearcher
type IndexTelemetry = search.IndexTelemetry

type SearchStatus string

const (
	SearchStatusIdle     SearchStatus = ""
	SearchStatusWalking  SearchStatus = "walking"
	SearchStatusIndex    SearchStatus = "index"
	SearchStatusMerging  SearchStatus = "merging"
	SearchStatusComplete SearchStatus = "complete"
)

// ===== STATE DEFINITIONS =====

// PreviewData contains preview information for selected file
type PreviewData struct {
	IsDir                      bool
	Name                       string
	Size                       int64
	Modified                   time.Time
	Mode                       os.FileMode
	LineCount                  int
	TextLines                  []string
	TextLineMeta               []TextLineMetadata
	FormattedTextLines         []string
	FormattedTextLineMeta      []TextLineMetadata
	FormattedSegments          [][]StyledTextSegment
	FormattedSegmentLineMeta   []TextLineMetadata
	FormattedKind              string
	FormattedUnavailableReason string
	TextCharCount              int
	TextTruncated              bool
	TextBytesRead              int64
	TextRemainder              []byte
	TextEncoding               fsutil.UnicodeEncoding
	BinaryInfo                 BinaryPreview
	DirEntries                 []FileEntry
	HiddenFormattingDetected   bool

	markdownDoc *markdownDocument
}

// BinaryPreview contains lightweight information about a binary file.
type BinaryPreview struct {
	Lines      []string
	ByteCount  int
	TotalBytes int64
}

// TextLineMetadata describes a preview line, including offsets for streaming.
type TextLineMetadata struct {
	Offset       int64
	Length       int
	RuneCount    int
	DisplayWidth int
}

// AppState is the single source of truth
type AppState struct {
	// Navigation & filesystem
	CurrentPath   string
	Files         []FileEntry // All files in current directory (always sorted)
	History       []string
	HistoryIndex  int
	ParentEntries []FileEntry // Entries from parent directory for sidebar

	// Directory loading
	DirectoryLoader          DirectoryLoader
	DirectoryLoading         bool
	DirectoryLoadingPath     string
	activeDirectoryLoadToken int
	directoryLoadSeq         int

	// Selection & viewport
	SelectedIndex int
	ScrollOffset  int

	// Filtering
	FilterActive        bool
	FilterQuery         string
	FilteredIndices     []int        // Indices into Files array
	FilterMatches       []FuzzyMatch // Match metadata aligned with FilteredIndices order
	FilterSavedIndex    int          // Saved selection index before entering filter mode
	FilterCaseSensitive bool
	filterMatcher       *FuzzyMatcher
	fileLowerNames      []string

	// Global search
	GlobalSearchActive               bool
	GlobalSearchQuery                string
	GlobalSearchCursorPos            int
	GlobalSearchCaseSensitive        bool
	GlobalSearchResults              []GlobalSearchResult
	GlobalSearchIndex                int // Selected result index
	GlobalSearchScroll               int
	GlobalSearchInProgress           bool // Whether search is still running
	GlobalSearchStatus               SearchStatus
	GlobalSearchRootPath             string // Where search started
	GlobalSearchID                   int    // Unique ID for current search (to cancel stale callbacks)
	GlobalSearcher                   *GlobalSearcher
	GlobalSearchIndexStatus          IndexTelemetry
	GlobalSearchDesiredSelectionPath string
	GlobalSearchPendingIndex         int
	GlobalSearchPendingIndexActive   bool
	LastGlobalSearchQuery            string
	LastGlobalSearchRootPath         string
	LastGlobalSearchIndex            int
	LastGlobalSearchScroll           int
	LastGlobalSearchSelectionPath    string
	dispatchAction                   func(Action)

	// Hidden files
	HideHiddenFiles bool // Whether to hide files starting with . (default true)

	// Preview
	PreviewData          *PreviewData
	PreviewPath          string
	PreviewFullScreen    bool
	PreviewWrap          bool
	PreviewScrollOffset  int
	PreviewWrapOffset    int
	PreviewPreferRaw     bool
	previewCache         map[string]previewCacheEntry
	previewScrollHistory map[string]previewScrollPosition
	previewDebounceTimer *time.Timer
	previewPendingToken  int
	previewPendingPath   string
	previewPendingReset  bool

	PreviewLoader          PreviewLoader
	PreviewLoading         bool
	PreviewLoadingPath     string
	activePreviewLoadToken int
	previewLoadSeq         int
	pendingPreviewReset    bool
	PreviewLoadingStarted  time.Time

	// Dimensions
	ScreenWidth  int
	ScreenHeight int

	// Status line
	ClipboardAvailable bool      // Whether clipboard command is available
	LastYankTime       time.Time // Time of last successful yank (for flash effect)
	EditorAvailable    bool      // Whether an editor command is available for 'e'

	// Error state
	LastError error

	// Display files cache (optimization to reduce allocations)
	displayFilesCache []FileEntry
	displayFilesDirty bool // True if cache is invalid
}

type filterToken struct {
	raw     string
	folded  string
	pattern string
	runes   []rune
}

type previewCacheEntry struct {
	size    int64
	modTime time.Time
	data    *PreviewData
}

type previewScrollPosition struct {
	scroll int
	wrap   int
}

func (s *AppState) nextDirectoryLoadToken() int {
	s.directoryLoadSeq++
	return s.directoryLoadSeq
}

func (s *AppState) ActiveDirectoryLoadToken() int {
	return s.activeDirectoryLoadToken
}

func (s *AppState) setDirectoryLoadInFlight(token int, path string) {
	s.activeDirectoryLoadToken = token
	if token == 0 {
		s.DirectoryLoading = false
		s.DirectoryLoadingPath = ""
		return
	}
	s.DirectoryLoading = true
	s.DirectoryLoadingPath = path
}

func (s *AppState) clearDirectoryLoadingState() {
	s.activeDirectoryLoadToken = 0
	s.DirectoryLoading = false
	s.DirectoryLoadingPath = ""
}

func (s *AppState) navigationPath() string {
	if s.DirectoryLoadingPath != "" {
		return s.DirectoryLoadingPath
	}
	return s.CurrentPath
}

func (s *AppState) nextPreviewLoadToken() int {
	s.previewLoadSeq++
	return s.previewLoadSeq
}

func (s *AppState) ActivePreviewLoadToken() int {
	return s.activePreviewLoadToken
}

func (s *AppState) setPreviewLoadInFlight(token int, path string, resetScroll bool) {
	s.activePreviewLoadToken = token
	if token == 0 {
		s.PreviewLoading = false
		s.PreviewLoadingPath = ""
		s.pendingPreviewReset = false
		s.PreviewLoadingStarted = time.Time{}
		return
	}
	s.PreviewLoading = true
	s.PreviewLoadingPath = path
	s.pendingPreviewReset = resetScroll
	s.PreviewLoadingStarted = time.Now()
}

func (s *AppState) clearPreviewLoadingState() {
	s.activePreviewLoadToken = 0
	s.PreviewLoading = false
	s.PreviewLoadingPath = ""
	s.pendingPreviewReset = false
	s.PreviewLoadingStarted = time.Time{}
}

func (s *AppState) previewShouldResetScroll() bool {
	return s.pendingPreviewReset
}

func (s *AppState) cancelPreviewDebounceTimer() {
	if s.previewDebounceTimer != nil {
		s.previewDebounceTimer.Stop()
		s.previewDebounceTimer = nil
	}
}

func (s *AppState) setPreviewPendingLoad(token int, path string, reset bool) {
	s.previewPendingToken = token
	s.previewPendingPath = path
	s.previewPendingReset = reset
}

func (s *AppState) previewPendingLoad() (int, string, bool) {
	return s.previewPendingToken, s.previewPendingPath, s.previewPendingReset
}

func (s *AppState) clearPreviewPendingLoad() {
	s.previewPendingToken = 0
	s.previewPendingPath = ""
	s.previewPendingReset = false
}

// ===== HELPER METHODS =====

// invalidateDisplayFilesCache marks the display files cache as dirty
// Should be called whenever FilterActive, FilteredIndices, or HideHiddenFiles changes
func (s *AppState) setDispatch(fn func(Action)) {
	s.dispatchAction = fn
}

func (s *AppState) getDispatch() func(Action) {
	return s.dispatchAction
}

// SetDispatch exposes the reducer dispatch hook to other packages.
func (s *AppState) SetDispatch(fn func(Action)) {
	s.setDispatch(fn)
}

func (s *AppState) refreshLowerNames() {
	if len(s.Files) == 0 {
		s.fileLowerNames = s.fileLowerNames[:0]
		return
	}
	if cap(s.fileLowerNames) < len(s.Files) {
		s.fileLowerNames = make([]string, len(s.Files))
	} else {
		s.fileLowerNames = s.fileLowerNames[:len(s.Files)]
	}
	for i, f := range s.Files {
		s.fileLowerNames[i] = strings.ToLower(f.Name)
	}
}

func (s *AppState) ensureLowerNames() {
	if len(s.fileLowerNames) != len(s.Files) {
		s.refreshLowerNames()
	}
}

// recomputeFilter rebuilds FilteredIndices based on FilterQuery using fuzzy matching
