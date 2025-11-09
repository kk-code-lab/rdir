package state

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/text/unicode/norm"
)

// LoadDirectory loads files from a directory into the provided AppState.
func LoadDirectory(state *AppState, path ...string) error {
	var dirPath string
	if len(path) > 0 {
		dirPath = path[0]
	} else {
		dirPath = state.CurrentPath
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("cannot read directory %s: %w", dirPath, err)
	}

	state.CurrentPath = dirPath
	state.Files = make([]FileEntry, len(entries))

	for i, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		isDir := e.IsDir()
		isSymlink := (info.Mode() & os.ModeSymlink) != 0

		// For symlinks, check if target is a directory
		if isSymlink {
			fullPath := filepath.Join(dirPath, e.Name())
			targetInfo, err := os.Stat(fullPath)
			if err == nil {
				isDir = targetInfo.IsDir()
			}
		}

		normalizedName := norm.NFC.String(e.Name())

		state.Files[i] = FileEntry{
			Name:      normalizedName,
			FullPath:  filepath.Join(dirPath, e.Name()),
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Size:      info.Size(),
			Modified:  info.ModTime(),
			Mode:      info.Mode(),
		}
	}

	state.sortFiles()
	state.resetViewport()
	state.updateParentEntries()

	return nil
}
