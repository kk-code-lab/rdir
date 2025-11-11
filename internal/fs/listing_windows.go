//go:build windows

package fs

// ShouldHideFromListing reports whether an entry should never appear in listings,
// even when hidden files are shown (e.g., Windows compatibility junctions).
func ShouldHideFromListing(fullPath, name string) bool {
	if fullPath == "" && name == "" {
		return false
	}

	attrs, err := getFileAttributes(fullPath, name)
	if err != nil {
		return false
	}

	const protectedMask = fileAttributeSystem | fileAttributeReparsePoint
	return attrs&protectedMask == protectedMask
}
