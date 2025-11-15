package state

import (
	"strconv"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

func bulletSymbol(depth int, ordered bool, idx int, start int) string {
	if ordered {
		if start <= 0 {
			start = 1
		}
		return strconv.Itoa(start+idx) + "."
	}
	switch depth {
	case 0:
		return "•"
	case 1:
		return "◦"
	default:
		return "▪"
	}
}

func displayWidthStr(s string) int {
	return textutil.DisplayWidth(s)
}
