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
