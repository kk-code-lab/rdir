package textutil

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

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
