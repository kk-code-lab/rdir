package state

import (
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/text/unicode/norm"
)

func (s *AppState) updateParentEntries() {
	parentPath := filepath.Dir(s.CurrentPath)
	if parentPath == "" || parentPath == s.CurrentPath {
		s.ParentEntries = nil
		return
	}

	currentName := filepath.Base(s.CurrentPath)

	entries, err := os.ReadDir(parentPath)
	if err != nil {
		s.ParentEntries = nil
		return
	}

	parentFiles := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		isDir := e.IsDir()
		isSymlink := (info.Mode() & os.ModeSymlink) != 0

		if isSymlink {
			targetInfo, err := os.Stat(filepath.Join(parentPath, e.Name()))
			if err == nil {
				isDir = targetInfo.IsDir()
			}
		}

		name := norm.NFC.String(e.Name())
		fullPath := filepath.Join(parentPath, e.Name())
		entry := FileEntry{
			Name:      name,
			FullPath:  fullPath,
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Size:      info.Size(),
			Modified:  info.ModTime(),
			Mode:      info.Mode(),
		}
		parentFiles = append(parentFiles, entry)
	}

	sort.Slice(parentFiles, func(i, j int) bool {
		if parentFiles[i].IsDir != parentFiles[j].IsDir {
			return parentFiles[i].IsDir
		}
		return parentFiles[i].Name < parentFiles[j].Name
	})

	if s.HideHiddenFiles {
		filtered := parentFiles[:0]
		for _, entry := range parentFiles {
			if entry.IsHidden() && entry.Name != currentName {
				continue
			}
			filtered = append(filtered, entry)
		}
		parentFiles = filtered
	}

	s.ParentEntries = parentFiles
}

func (s *AppState) RefreshParentEntries() {
	s.updateParentEntries()
}

func (s *AppState) sortFiles() {
	sort.Slice(s.Files, func(i, j int) bool {
		if s.Files[i].IsDir != s.Files[j].IsDir {
			return s.Files[i].IsDir
		}
		return s.Files[i].Name < s.Files[j].Name
	})
}

func (s *AppState) resetViewport() {
	s.SelectedIndex = 0
	s.ScrollOffset = 0
	s.clearFilter()

	if s.HideHiddenFiles && len(s.Files) > 0 && s.Files[0].IsHidden() {
		for i, f := range s.Files {
			if !f.IsHidden() {
				s.SelectedIndex = i
				break
			}
		}
	}
}

func (s *AppState) getCurrentFile() *FileEntry {
	displayFiles := s.getDisplayFiles()
	displayIdx := s.getDisplaySelectedIndex()

	if displayIdx >= 0 && displayIdx < len(displayFiles) {
		return &displayFiles[displayIdx]
	}
	return nil
}

func (s *AppState) CurrentFile() *FileEntry {
	return s.getCurrentFile()
}

func (s *AppState) getCurrentFilePath() string {
	file := s.getCurrentFile()
	current := s.CurrentPath
	if current == "" {
		current = "."
	}
	if file != nil {
		current = filepath.Join(current, file.Name)
	}
	return filepath.Clean(current)
}

func (s *AppState) CurrentFilePath() string {
	return s.getCurrentFilePath()
}

func (s *AppState) getSymlinkTarget() string {
	file := s.getCurrentFile()
	if file == nil || !file.IsSymlink {
		return ""
	}

	filePath := s.getCurrentFilePath()

	target, err := os.Readlink(filePath)
	if err != nil {
		return ""
	}

	return target
}

func (s *AppState) SymlinkTarget() string {
	return s.getSymlinkTarget()
}
