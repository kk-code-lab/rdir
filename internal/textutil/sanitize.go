package textutil

import "strings"

var formattingRuneLabels = map[rune]string{
	0x061C: "⟪ALM⟫",
	0x200B: "⟪ZWSP⟫",
	0x200C: "⟪ZWNJ⟫",
	0x200D: "⟪ZWJ⟫",
	0x200E: "⟪LRM⟫",
	0x200F: "⟪RLM⟫",
	0x202A: "⟪LRE⟫",
	0x202B: "⟪RLE⟫",
	0x202C: "⟪PDF⟫",
	0x202D: "⟪LRO⟫",
	0x202E: "⟪RLO⟫",
	0x2028: "⟪LSEP⟫",
	0x2029: "⟪PSEP⟫",
	0x00AD: "⟪SHY⟫",
	0x180E: "⟪MVS⟫",
	0x2060: "⟪WJ⟫",
	0x2066: "⟪LRI⟫",
	0x2067: "⟪RLI⟫",
	0x2068: "⟪FSI⟫",
	0x2069: "⟪PDI⟫",
	0x206A: "⟪ISS⟫",
	0x206B: "⟪ASS⟫",
	0x206C: "⟪IAFS⟫",
	0x206D: "⟪AAFS⟫",
	0x206E: "⟪NADS⟫",
	0x206F: "⟪NODS⟫",
	0xFEFF: "⟪BOM⟫",
}

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
	if isFormattingRune(r) {
		return true
	}
	return (r >= 0 && r < 0x20) || r == 0x7f
}

func sanitize(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case isFormattingRune(r):
			b.WriteString(formattingRuneLabels[r])
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

// ReplaceFormattingRunes makes bidi/zero-width formatting characters visible.
// It returns the rewritten string and whether any replacement occurred.
func ReplaceFormattingRunes(text string) (string, bool) {
	var b strings.Builder
	changed := false
	for _, r := range text {
		if label, ok := formattingRuneLabels[r]; ok {
			changed = true
			b.WriteString(label)
			continue
		}
		b.WriteRune(r)
	}
	if !changed {
		return text, false
	}
	return b.String(), true
}

// HasFormattingRunes reports whether text contains bidi or zero-width formatting runes.
func HasFormattingRunes(text string) bool {
	for _, r := range text {
		if isFormattingRune(r) {
			return true
		}
	}
	return false
}

func isFormattingRune(r rune) bool {
	_, ok := formattingRuneLabels[r]
	return ok
}
