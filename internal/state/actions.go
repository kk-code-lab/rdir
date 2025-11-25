package state

import "os"

// Action is the base interface for all state mutations
type Action interface{}

// ===== NAVIGATION ACTIONS =====

type NavigateUpAction struct{}
type NavigateDownAction struct{}
type EnterDirectoryAction struct{}
type RightArrowAction struct{}
type GoUpAction struct{}
type GoHomeAction struct{}
type GoToHistoryAction struct {
	Direction string // "back" or "forward"
}

// ===== FILTER ACTIONS =====

type FilterStartAction struct{}
type FilterCharAction struct {
	Char rune
}
type FilterBackspaceAction struct{}
type FilterResetQueryAction struct{}
type FilterClearAction struct{}

// ===== SCROLL ACTIONS =====

type ScrollUpAction struct{}
type ScrollDownAction struct{}
type ScrollPageUpAction struct{}
type ScrollPageDownAction struct{}
type ScrollToStartAction struct{}
type ScrollToEndAction struct{}

// ===== VIEW ACTIONS =====

// MouseSelectAction selects a row in the main file list by its display index.
type MouseSelectAction struct {
	DisplayIndex int
}

type ResizeAction struct {
	Width  int
	Height int
}

type YankPathAction struct{}
type ToggleHiddenFilesAction struct{}
type OpenEditorAction struct{}
type RefreshDirectoryAction struct{}
type OpenPagerAction struct{}
type GoToPathAction struct {
	Path string
}
type HelpToggleAction struct{}
type HelpHideAction struct{}

// DirectoryLoadResultAction installs results from the async directory loader.
type DirectoryLoadResultAction struct {
	Token   int
	Path    string
	Entries []FileEntry
	Err     error
}

// ===== PREVIEW ACTIONS =====

type PreviewEnterFullScreenAction struct{}
type PreviewExitFullScreenAction struct{}
type PreviewScrollUpAction struct{}
type PreviewScrollDownAction struct{}
type PreviewScrollPageUpAction struct{}
type PreviewScrollPageDownAction struct{}
type PreviewScrollToStartAction struct{}
type PreviewScrollToEndAction struct{}
type TogglePreviewWrapAction struct{}
type PreviewLoadStartAction struct {
	Token int
}
type PreviewLoadResultAction struct {
	Token   int
	Path    string
	Preview *PreviewData
	Info    os.FileInfo
	Err     error
}

// ===== GLOBAL SEARCH ACTIONS =====

type GlobalSearchStartAction struct{}
type GlobalSearchCharAction struct {
	Char rune
}
type GlobalSearchBackspaceAction struct{}
type GlobalSearchDeleteAction struct{}
type GlobalSearchDeleteWordAction struct{}
type GlobalSearchResetQueryAction struct{}
type GlobalSearchMoveCursorAction struct {
	Direction string // "left", "right", "home", "end"
}
type GlobalSearchClearAction struct{}
type GlobalSearchNavigateAction struct {
	Direction string // "up" or "down"
}

// GlobalSearchSelectIndexAction selects a specific result row (0-based).
type GlobalSearchSelectIndexAction struct {
	Index int
}
type GlobalSearchPageUpAction struct{}
type GlobalSearchPageDownAction struct{}
type GlobalSearchHomeAction struct{}
type GlobalSearchEndAction struct{}
type GlobalSearchOpenAction struct{}
type GlobalSearchIndexProgressAction struct {
	Progress IndexTelemetry
}
type GlobalSearchResultsAction struct {
	Results    []GlobalSearchResult
	InProgress bool
	Phase      SearchStatus
}

// ===== APPLICATION ACTIONS =====

type QuitAction struct{}          // q - return to original directory
type QuitAndChangeAction struct{} // x - change to current directory
