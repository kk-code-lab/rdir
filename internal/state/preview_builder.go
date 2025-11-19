package state

import (
	"os"
	"path/filepath"
	"sort"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	"golang.org/x/text/unicode/norm"
)

func buildPreviewData(filePath string, hideHidden bool) (*PreviewData, os.FileInfo, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, err
	}

	normalizedName := norm.NFC.String(info.Name())
	preview := &PreviewData{
		IsDir:    info.IsDir(),
		Name:     normalizedName,
		Size:     info.Size(),
		Modified: info.ModTime(),
		Mode:     info.Mode(),
	}

	if info.IsDir() {
		loadDirectoryPreview(preview, filePath, hideHidden)
	} else {
		loadFilePreview(preview, filePath, info)
	}

	return preview, info, nil
}

func loadDirectoryPreview(preview *PreviewData, filePath string, hideHidden bool) {
	entries, err := os.ReadDir(filePath)
	if err != nil {
		return
	}

	for _, e := range entries {
		entryInfo, err := e.Info()
		if err != nil {
			continue
		}

		isDir := e.IsDir()
		isSymlink := (entryInfo.Mode() & os.ModeSymlink) != 0
		if isSymlink {
			targetInfo, err := os.Stat(filepath.Join(filePath, e.Name()))
			if err == nil {
				isDir = targetInfo.IsDir()
			}
		}

		normalizedName := norm.NFC.String(e.Name())
		entry := FileEntry{
			Name:      normalizedName,
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Size:      entryInfo.Size(),
			Modified:  entryInfo.ModTime(),
			Mode:      entryInfo.Mode(),
		}

		if hideHidden && entry.IsHidden() {
			continue
		}

		preview.DirEntries = append(preview.DirEntries, entry)
	}

	sort.Slice(preview.DirEntries, func(i, j int) bool {
		if preview.DirEntries[i].IsDir != preview.DirEntries[j].IsDir {
			return preview.DirEntries[i].IsDir
		}
		return preview.DirEntries[i].Name < preview.DirEntries[j].Name
	})
}

func loadFilePreview(preview *PreviewData, filePath string, info os.FileInfo) {
	content, err := fsutil.ReadFileHead(filePath, previewByteLimit)
	if err != nil {
		return
	}

	ctx := previewFormatContext{
		path:    filePath,
		info:    info,
		content: content,
	}
	for _, formatter := range previewFormatters {
		if formatter.CanHandle(ctx) {
			formatter.Format(ctx, preview)
			break
		}
	}
}
