//go:build windows

package fs

import (
	"os"
	"syscall"
)

const (
	fileAttributeHidden       = 0x02
	fileAttributeSystem       = 0x04
	fileAttributeReparsePoint = 0x0400
)

// getFileAttributes resolves Windows file attributes for the provided path/name.
// Returns an error if the attributes cannot be read.
func getFileAttributes(fullPath, name string) (uint32, error) {
	target := fullPath
	if target == "" {
		target = name
	}
	if target == "" {
		return 0, os.ErrInvalid
	}

	ptr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return 0, err
	}

	attrs, err := syscall.GetFileAttributes(ptr)
	if err == nil {
		return attrs, nil
	}

	if os.IsNotExist(err) && fullPath != "" && fullPath != name {
		ptrAlt, convErr := syscall.UTF16PtrFromString(name)
		if convErr == nil {
			if attrsAlt, errAlt := syscall.GetFileAttributes(ptrAlt); errAlt == nil {
				return attrsAlt, nil
			}
		}
	}

	return 0, err
}
