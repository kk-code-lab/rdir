//go:build !windows

package shellsetup

func DetectParentShellName() string {
	return ""
}
