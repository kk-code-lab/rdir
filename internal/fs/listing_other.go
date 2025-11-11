//go:build !windows

package fs

// ShouldHideFromListing is a no-op on non-Windows platforms.
func ShouldHideFromListing(_, _ string) bool {
	return false
}
