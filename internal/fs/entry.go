package fs

import (
	"os"
	"time"
)

// Entry represents a single file or directory on disk.
type Entry struct {
	Name      string
	FullPath  string
	IsDir     bool
	IsSymlink bool
	Size      int64
	Modified  time.Time
	Mode      os.FileMode
}

// IsHidden reports whether the entry should be treated as hidden.
func (e Entry) IsHidden() bool {
	return IsHidden(e.FullPath, e.Name)
}
