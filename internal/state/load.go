package state

import (
	"fmt"
	"os"
	"path/filepath"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
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
	visibleEntries := make([]FileEntry, 0, len(entries))

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		rawName := e.Name()
		fullPath := filepath.Join(dirPath, rawName)

		if fsutil.ShouldHideFromListing(fullPath, rawName) {
			continue
		}

		isDir := e.IsDir()
		isSymlink := (info.Mode() & os.ModeSymlink) != 0

		// For symlinks, check if target is a directory
		if isSymlink {
			targetInfo, err := os.Stat(fullPath)
			if err == nil {
				isDir = targetInfo.IsDir()
			}
		}

		normalizedName := norm.NFC.String(rawName)

		visibleEntries = append(visibleEntries, FileEntry{
			Name:      normalizedName,
			FullPath:  fullPath,
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Size:      info.Size(),
			Modified:  info.ModTime(),
			Mode:      info.Mode(),
		})
	}

	state.Files = visibleEntries

	state.sortFiles()
	state.resetViewport()
	state.updateParentEntries()

	return nil
}
