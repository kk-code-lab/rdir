package state

// invalidateDisplayFilesCache marks the display files cache as dirty.
func (s *AppState) invalidateDisplayFilesCache() {
	s.displayFilesDirty = true
	s.displayFilesCache = nil
}

func (s *AppState) getDisplayFiles() []FileEntry {
	if !s.displayFilesDirty && s.displayFilesCache != nil {
		result := make([]FileEntry, len(s.displayFilesCache))
		copy(result, s.displayFilesCache)
		return result
	}

	var files []FileEntry

	if s.FilterActive {
		for _, idx := range s.FilteredIndices {
			if idx >= 0 && idx < len(s.Files) {
				files = append(files, s.Files[idx])
			}
		}
	} else {
		files = s.Files
	}

	if s.HideHiddenFiles {
		var visible []FileEntry
		for _, f := range files {
			if !f.IsHidden() {
				visible = append(visible, f)
			}
		}
		files = visible
	}

	s.displayFilesCache = files
	s.displayFilesDirty = false

	result := make([]FileEntry, len(files))
	copy(result, files)
	return result
}

func (s *AppState) DisplayFiles() []FileEntry {
	return s.getDisplayFiles()
}

func (s *AppState) getDisplaySelectedIndex() int {
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Files) {
		return -1
	}

	if s.FilterActive {
		displayIdx := 0
		for _, fileIdx := range s.FilteredIndices {
			if fileIdx == s.SelectedIndex {
				if s.HideHiddenFiles {
					visibleCount := 0
					for _, idx := range s.FilteredIndices {
						if idx == s.SelectedIndex {
							return visibleCount
						}
						if !s.Files[idx].IsHidden() {
							visibleCount++
						}
					}
				}
				return displayIdx
			}
			if !s.HideHiddenFiles || !s.Files[fileIdx].IsHidden() {
				displayIdx++
			}
		}
		return -1
	}

	if s.HideHiddenFiles {
		if s.Files[s.SelectedIndex].IsHidden() {
			return -1
		}
		displayIdx := 0
		for i := 0; i < s.SelectedIndex; i++ {
			if !s.Files[i].IsHidden() {
				displayIdx++
			}
		}
		return displayIdx
	}

	return s.SelectedIndex
}

func (s *AppState) DisplaySelectedIndex() int {
	return s.getDisplaySelectedIndex()
}

func (s *AppState) getActualIndexFromDisplayIndex(displayIdx int) int {
	displayFiles := s.getDisplayFiles()

	if displayIdx < 0 || displayIdx >= len(displayFiles) {
		return -1
	}

	if s.FilterActive && s.HideHiddenFiles {
		visibleCount := 0
		for _, fileIdx := range s.FilteredIndices {
			if !s.Files[fileIdx].IsHidden() {
				if visibleCount == displayIdx {
					return fileIdx
				}
				visibleCount++
			}
		}
	} else if s.FilterActive {
		if displayIdx < len(s.FilteredIndices) {
			return s.FilteredIndices[displayIdx]
		}
	} else if s.HideHiddenFiles {
		visibleCount := 0
		for i := 0; i < len(s.Files); i++ {
			if !s.Files[i].IsHidden() {
				if visibleCount == displayIdx {
					return i
				}
				visibleCount++
			}
		}
	} else {
		return displayIdx
	}

	return -1
}

func (s *AppState) ActualIndexFromDisplayIndex(displayIdx int) int {
	return s.getActualIndexFromDisplayIndex(displayIdx)
}

func (s *AppState) setDisplaySelectedIndex(displayIdx int) {
	if s.FilterActive {
		if s.HideHiddenFiles {
			visibleCount := 0
			for _, fileIdx := range s.FilteredIndices {
				if s.Files[fileIdx].IsHidden() {
					continue
				}
				if visibleCount == displayIdx {
					s.SelectedIndex = fileIdx
					return
				}
				visibleCount++
			}
		} else {
			if displayIdx < len(s.FilteredIndices) {
				s.SelectedIndex = s.FilteredIndices[displayIdx]
			}
		}
		return
	}

	if s.HideHiddenFiles {
		visibleCount := 0
		for i := 0; i < len(s.Files); i++ {
			if !s.Files[i].IsHidden() {
				if visibleCount == displayIdx {
					s.SelectedIndex = i
					return
				}
				visibleCount++
			}
		}
	} else {
		s.SelectedIndex = displayIdx
	}
}

func (s *AppState) updateScrollVisibility() {
	displayIdx := s.getDisplaySelectedIndex()
	visibleLines := s.ScreenHeight - 4

	if displayIdx < 0 {
		return
	}

	if displayIdx < s.ScrollOffset {
		s.ScrollOffset = displayIdx
	} else if displayIdx >= s.ScrollOffset+visibleLines {
		s.ScrollOffset = displayIdx - visibleLines + 1
	}

	displayFiles := s.getDisplayFiles()
	maxOffset := len(displayFiles) - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.ScrollOffset < 0 {
		s.ScrollOffset = 0
	}
	if s.ScrollOffset > maxOffset {
		s.ScrollOffset = maxOffset
	}
}

func (s *AppState) centerScrollOnSelection() {
	displayIdx := s.getDisplaySelectedIndex()
	visibleLines := s.ScreenHeight - 4

	if displayIdx < 0 {
		return
	}

	s.ScrollOffset = displayIdx - visibleLines/2

	displayFiles := s.getDisplayFiles()
	maxOffset := len(displayFiles) - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.ScrollOffset < 0 {
		s.ScrollOffset = 0
	}
	if s.ScrollOffset > maxOffset {
		s.ScrollOffset = maxOffset
	}
}
