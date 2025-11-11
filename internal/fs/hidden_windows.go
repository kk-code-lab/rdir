//go:build windows

package fs

// IsHidden checks if a file is hidden on this platform (Windows)
func IsHidden(fullPath string, name string) bool {
	if fullPath == "" && name == "" {
		return false
	}

	attrs, err := getFileAttributes(fullPath, name)
	if err != nil {
		return len(name) > 0 && name[0] == '.'
	}

	return attrs&fileAttributeHidden != 0
}
