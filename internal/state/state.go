package state

import (
	"os"
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
	IsDir      bool
	Name       string
	Size       int64
	Modified   time.Time
	Mode       os.FileMode
	LineCount  int
	TextLines  []string
	BinaryInfo BinaryPreview
	DirEntries []FileEntry
}

// BinaryPreview contains lightweight information about a binary file.
type BinaryPreview struct {
	Lines      []string
	ByteCount  int
	TotalBytes int64
}

// AppState is the single source of truth
type AppState struct {
	// Navigation & filesystem
	CurrentPath   string
	Files         []FileEntry // All files in current directory (always sorted)
	History       []string
	HistoryIndex  int
	ParentEntries []FileEntry // Entries from parent directory for sidebar

	// Selection & viewport
	SelectedIndex int
	ScrollOffset  int

	// Filtering
	FilterActive        bool
	FilterQuery         string
	FilteredIndices     []int        // Indices into Files array
	FilterMatches       []FuzzyMatch // Sorted matches with scores (new fuzzy search)
	FilterSavedIndex    int          // Saved selection index before entering filter mode
	FilterCaseSensitive bool
	filterMatcher       *FuzzyMatcher

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
	PreviewData         *PreviewData
	PreviewFullScreen   bool
	PreviewWrap         bool
	PreviewScrollOffset int
	previewCache        map[string]previewCacheEntry

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

// recomputeFilter rebuilds FilteredIndices based on FilterQuery using fuzzy matching
