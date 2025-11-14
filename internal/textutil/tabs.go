package textutil

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

const DefaultTabWidth = 4

// ExpandTabs replaces tab characters with spaces respecting terminal column width.
func ExpandTabs(text string, tabWidth int) string {
	if tabWidth <= 0 || !strings.ContainsRune(text, '\t') {
		return text
	}

	var builder strings.Builder
	column := 0
	for _, ru := range text {
		if ru == '\t' {
			spaces := tabWidth - (column % tabWidth)
			for i := 0; i < spaces; i++ {
				builder.WriteByte(' ')
			}
			column += spaces
			continue
		}
		builder.WriteRune(ru)
		width := runewidth.RuneWidth(ru)
		if width < 1 {
			width = 1
		}
		column += width
	}
	return builder.String()
}

// DisplayWidth reports the printable width of text accounting for wide runes.
func DisplayWidth(text string) int {
	width := 0
	for _, ru := range text {
		w := runewidth.RuneWidth(ru)
		if w <= 0 {
			w = 1
		}
		width += w
	}
	return width
}
