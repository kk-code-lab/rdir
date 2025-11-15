package textutil

import "strings"

// SanitizeTerminalText replaces control characters so user-controlled text cannot
// inject terminal escape sequences when rendered.
func SanitizeTerminalText(text string) string {
	for _, r := range text {
		if requiresSanitization(r) {
			return sanitize(text)
		}
	}
	return text
}

func requiresSanitization(r rune) bool {
	if r == '\t' {
		return false
	}
	if r == '\n' || r == '\r' {
		return true
	}
	return (r >= 0 && r < 0x20) || r == 0x7f
}

func sanitize(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '\t', r == '\n', r == '\r':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
