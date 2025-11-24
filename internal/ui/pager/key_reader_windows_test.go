//go:build windows

package pager

import "testing"

func TestRuneToPagerKeyStartsBinarySearch(t *testing.T) {
	ev, ok := runeToPagerKey(':')
	if !ok {
		t.Fatalf("expected ':' to map to a pager key")
	}
	if ev.kind != keyStartBinarySearch {
		t.Fatalf("expected keyStartBinarySearch, got %v", ev.kind)
	}
	if ev.ch != ':' {
		t.Fatalf("expected ':' rune to be set, got %q", ev.ch)
	}
}

func TestRuneToPagerKeyCtrlShortcuts(t *testing.T) {
	ev, ok := runeToPagerKey(0x02) // Ctrl+B
	if !ok || ev.kind != keyToggleBinarySearchMode {
		t.Fatalf("ctrl+b should toggle binary search mode, got %+v (ok=%v)", ev, ok)
	}

	ev, ok = runeToPagerKey(0x0c) // Ctrl+L
	if !ok || ev.kind != keyToggleBinarySearchLimit {
		t.Fatalf("ctrl+l should toggle binary search limit, got %+v (ok=%v)", ev, ok)
	}
}
