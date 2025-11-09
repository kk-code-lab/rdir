//go:build !windows

package fs

// IsHidden checks if a file is hidden on this platform (Unix-like)
func IsHidden(_ string, name string) bool {
	return len(name) > 0 && name[0] == '.'
}
