package state

import "os"

func clonePreviewData(src *PreviewData) *PreviewData {
	if src == nil {
		return nil
	}

	copyData := *src
	if len(src.TextLines) > 0 {
		copyData.TextLines = append([]string(nil), src.TextLines...)
	}
	if len(src.TextLineMeta) > 0 {
		copyData.TextLineMeta = append([]TextLineMetadata(nil), src.TextLineMeta...)
	}
	if len(src.TextRemainder) > 0 {
		copyData.TextRemainder = append([]byte(nil), src.TextRemainder...)
	}
	if len(src.BinaryInfo.Lines) > 0 {
		copyData.BinaryInfo.Lines = append([]string(nil), src.BinaryInfo.Lines...)
	}
	if len(src.DirEntries) > 0 {
		copyData.DirEntries = append([]FileEntry(nil), src.DirEntries...)
	}
	return &copyData
}

func (s *AppState) getCachedFilePreview(path string, info os.FileInfo) (*PreviewData, bool) {
	if s.previewCache == nil {
		return nil, false
	}

	entry, ok := s.previewCache[path]
	if !ok {
		return nil, false
	}

	if entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
		return clonePreviewData(entry.data), true
	}

	return nil, false
}

func (s *AppState) storeFilePreview(path string, info os.FileInfo, data *PreviewData) {
	if data == nil {
		return
	}
	if s.previewCache == nil {
		s.previewCache = make(map[string]previewCacheEntry)
	}
	s.previewCache[path] = previewCacheEntry{
		size:    info.Size(),
		modTime: info.ModTime(),
		data:    clonePreviewData(data),
	}
}

func (s *AppState) rememberPreviewScrollForCurrentFile() {
	if s == nil {
		return
	}
	path := s.CurrentFilePath()
	if path == "" {
		return
	}
	if s.previewScrollHistory == nil {
		s.previewScrollHistory = make(map[string]previewScrollPosition)
	}
	s.previewScrollHistory[path] = previewScrollPosition{
		scroll: s.PreviewScrollOffset,
		wrap:   s.PreviewWrapOffset,
	}
}

func (s *AppState) restorePreviewScrollForPath(path string) bool {
	if s == nil || path == "" || s.previewScrollHistory == nil {
		return false
	}
	pos, ok := s.previewScrollHistory[path]
	if !ok {
		return false
	}
	s.PreviewScrollOffset = pos.scroll
	s.PreviewWrapOffset = pos.wrap
	return true
}

func (s *AppState) previewLineCount() int {
	if s == nil || s.PreviewData == nil {
		return 0
	}
	if s.PreviewData.LineCount > 0 {
		return s.PreviewData.LineCount
	}
	switch {
	case s.PreviewData.IsDir:
		return len(s.PreviewData.DirEntries)
	case len(s.PreviewData.TextLines) > 0:
		return len(s.PreviewData.TextLines)
	case len(s.PreviewData.BinaryInfo.Lines) > 0:
		return len(s.PreviewData.BinaryInfo.Lines)
	default:
		return 0
	}
}

func (s *AppState) previewVisibleLines() int {
	if s == nil {
		return 0
	}
	lines := s.ScreenHeight - 2
	if s.PreviewFullScreen {
		lines = s.ScreenHeight - 4
	}
	if lines < 0 {
		lines = 0
	}
	return lines
}

func (s *AppState) maxPreviewScrollOffset() int {
	lines := s.previewLineCount()
	visible := s.previewVisibleLines()
	if visible <= 0 || lines <= visible {
		return 0
	}
	return lines - visible
}

func (s *AppState) clampPreviewScroll() {
	if s == nil {
		return
	}
	if s.PreviewScrollOffset < 0 {
		s.PreviewScrollOffset = 0
		return
	}
	maxOffset := s.maxPreviewScrollOffset()
	if s.PreviewScrollOffset > maxOffset {
		s.PreviewScrollOffset = maxOffset
	}
}

func (s *AppState) resetPreviewScroll() {
	if s == nil {
		return
	}
	s.PreviewScrollOffset = 0
	s.PreviewWrapOffset = 0
	if s.PreviewData == nil {
		s.PreviewFullScreen = false
		s.PreviewWrap = false
	}
}

func (s *AppState) scrollPreviewBy(delta int) {
	if s == nil || delta == 0 {
		return
	}
	s.PreviewScrollOffset += delta
	s.clampPreviewScroll()
}

func (s *AppState) normalizePreviewScroll() {
	if s == nil {
		return
	}
	total := s.previewLineCount()
	visible := s.previewVisibleLines()
	if visible <= 0 || total <= 0 {
		return
	}
	if s.PreviewScrollOffset > total-visible {
		s.PreviewScrollOffset = total - visible
		if s.PreviewScrollOffset < 0 {
			s.PreviewScrollOffset = 0
		}
	}
}
