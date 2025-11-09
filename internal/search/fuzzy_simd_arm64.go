//go:build arm64 && !purego

package search

import (
	"unsafe"

	"golang.org/x/sys/cpu"
)

func lowerASCIIUsingPlatform(dst []rune, src string) bool {
	if len(dst) == 0 || len(src) == 0 {
		return len(src) == 0
	}
	if len(dst) < len(src) || !cpu.ARM64.HasASIMD {
		return false
	}
	lowerASCIIASIMD((*uint32)(unsafe.Pointer(&dst[0])), unsafe.StringData(src), len(src))
	return true
}

//go:noescape
func lowerASCIIASIMD(dst *uint32, src *byte, n int)
