//go:build windows

package fs

import (
	"os"
	"syscall"
)

// IsHidden checks if a file is hidden on this platform (Windows)
func IsHidden(fullPath string, name string) bool {
	target := fullPath
	if target == "" {
		target = name
	}
	if target == "" {
		return false
	}

	ptr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return len(name) > 0 && name[0] == '.'
	}

	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		if os.IsNotExist(err) && fullPath != name && fullPath != "" {
			ptrAlt, convErr := syscall.UTF16PtrFromString(name)
			if convErr == nil {
				if attrsAlt, errAlt := syscall.GetFileAttributes(ptrAlt); errAlt == nil {
					attrs = attrsAlt
					err = nil
				}
			}
		}
		if err != nil {
			return len(name) > 0 && name[0] == '.'
		}
	}
	const FILE_ATTRIBUTE_HIDDEN = 0x02
	return attrs&FILE_ATTRIBUTE_HIDDEN != 0
}
