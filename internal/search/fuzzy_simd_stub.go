//go:build !arm64 || purego

package search

func lowerASCIIUsingPlatform(dst []rune, src string) bool {
	return false
}
