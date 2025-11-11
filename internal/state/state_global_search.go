package state

import "strings"

func (s *AppState) setGlobalSearchQuery(value string) {
	s.GlobalSearchQuery = value
}

func (s *AppState) CleanGlobalSearchQuery() string {
	return strings.TrimSpace(s.GlobalSearchQuery)
}

func (s *AppState) rememberGlobalSearchQuery() {
	if s.CleanGlobalSearchQuery() == "" {
		return
	}

	s.LastGlobalSearchQuery = s.GlobalSearchQuery
	s.LastGlobalSearchRootPath = s.GlobalSearchRootPath
	s.LastGlobalSearchIndex = s.GlobalSearchIndex
	s.LastGlobalSearchScroll = s.GlobalSearchScroll
	if idx := s.GlobalSearchIndex; idx >= 0 && idx < len(s.GlobalSearchResults) {
		s.LastGlobalSearchSelectionPath = s.GlobalSearchResults[idx].FilePath
	} else {
		s.LastGlobalSearchSelectionPath = ""
	}
}

func (s *AppState) forgetGlobalSearchMemory() {
	s.LastGlobalSearchQuery = ""
	s.LastGlobalSearchRootPath = ""
	s.LastGlobalSearchIndex = 0
	s.LastGlobalSearchScroll = 0
	s.LastGlobalSearchSelectionPath = ""
}

func (s *AppState) clearGlobalSearch(forgetMemory bool) {
	if forgetMemory {
		s.forgetGlobalSearchMemory()
	} else {
		s.rememberGlobalSearchQuery()
	}

	s.GlobalSearchActive = false
	s.setGlobalSearchQuery("")
	s.GlobalSearchCursorPos = 0
	s.GlobalSearchCaseSensitive = false
	s.GlobalSearchResults = nil
	s.GlobalSearchIndex = 0
	s.GlobalSearchScroll = 0
	s.GlobalSearchInProgress = false
	s.GlobalSearchStatus = SearchStatusIdle
	s.GlobalSearchRootPath = ""
	s.GlobalSearchDesiredSelectionPath = ""
	s.clearGlobalSearchPendingIndex()
	s.GlobalSearchIndexStatus = IndexTelemetry{}
}

func (s *AppState) clampGlobalSearchSelection() {
	if len(s.GlobalSearchResults) == 0 {
		s.GlobalSearchIndex = 0
		s.GlobalSearchScroll = 0
		return
	}

	if s.GlobalSearchIndex < 0 {
		s.GlobalSearchIndex = 0
	} else if s.GlobalSearchIndex >= len(s.GlobalSearchResults) {
		s.GlobalSearchIndex = len(s.GlobalSearchResults) - 1
	}

	s.updateGlobalSearchScroll()
}

func (s *AppState) applyDesiredGlobalSearchSelection() {
	if s.GlobalSearchDesiredSelectionPath == "" {
		return
	}

	for idx, result := range s.GlobalSearchResults {
		if result.FilePath == s.GlobalSearchDesiredSelectionPath {
			s.GlobalSearchIndex = idx
			s.updateGlobalSearchScroll()
			s.GlobalSearchDesiredSelectionPath = ""
			return
		}
	}
}

func (s *AppState) clearDesiredGlobalSearchSelection() {
	s.GlobalSearchDesiredSelectionPath = ""
}

func (s *AppState) applyPendingGlobalSearchIndex() {
	if !s.GlobalSearchPendingIndexActive {
		return
	}

	if s.GlobalSearchPendingIndex >= 0 && s.GlobalSearchPendingIndex < len(s.GlobalSearchResults) {
		s.GlobalSearchIndex = s.GlobalSearchPendingIndex
		s.updateGlobalSearchScroll()
		s.clearGlobalSearchPendingIndex()
	}
}

func (s *AppState) setGlobalSearchPendingIndex(idx int) {
	if idx < 0 {
		s.clearGlobalSearchPendingIndex()
		return
	}
	s.GlobalSearchPendingIndex = idx
	s.GlobalSearchPendingIndexActive = true
}

func (s *AppState) clearGlobalSearchPendingIndex() {
	s.GlobalSearchPendingIndex = 0
	s.GlobalSearchPendingIndexActive = false
}

func (s *AppState) updateGlobalSearchScroll() {
	visibleLines := s.ScreenHeight - 4
	if visibleLines < 1 {
		visibleLines = 1
	}

	maxScroll := len(s.GlobalSearchResults) - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	if s.GlobalSearchScroll < 0 {
		s.GlobalSearchScroll = 0
	}
	if s.GlobalSearchScroll > maxScroll {
		s.GlobalSearchScroll = maxScroll
	}

	if s.GlobalSearchIndex < s.GlobalSearchScroll {
		s.GlobalSearchScroll = s.GlobalSearchIndex
	} else if s.GlobalSearchIndex >= s.GlobalSearchScroll+visibleLines {
		s.GlobalSearchScroll = s.GlobalSearchIndex - visibleLines + 1
	}
}

func (s *AppState) restoreGlobalSearchSelection(prevResults []GlobalSearchResult, prevIndex int) {
	if len(s.GlobalSearchResults) == 0 {
		s.GlobalSearchIndex = 0
		s.GlobalSearchScroll = 0
		return
	}

	if prevIndex < 0 {
		s.GlobalSearchIndex = 0
		s.updateGlobalSearchScroll()
		return
	}

	if prevIndex >= len(s.GlobalSearchResults) {
		s.GlobalSearchIndex = len(s.GlobalSearchResults) - 1
		s.updateGlobalSearchScroll()
		return
	}

	s.GlobalSearchIndex = prevIndex
	s.updateGlobalSearchScroll()
}

func (s *AppState) SearchStatusLabel() string {
	return s.GlobalSearchStatus.label(s.GlobalSearchInProgress)
}

func (status SearchStatus) label(inProgress bool) string {
	switch status {
	case SearchStatusWalking:
		if inProgress {
			return "walking filesystem…"
		}
		return "filesystem scan complete"
	case SearchStatusIndex:
		if inProgress {
			return "querying index…"
		}
		return "index lookup complete"
	case SearchStatusMerging:
		if inProgress {
			return "merging results…"
		}
		return "merge complete"
	case SearchStatusComplete:
		return "results ready"
	default:
		if inProgress {
			return "searching…"
		}
		return ""
	}
}

func (s *AppState) CurrentIndexStatus() IndexTelemetry {
	return s.GlobalSearchIndexStatus
}
