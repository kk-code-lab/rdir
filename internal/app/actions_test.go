package app

import "testing"

func TestNormalizeClipboardPathWindows(t *testing.T) {
	input := `C:\Users\me/project/sub/file.txt`
	got := normalizeClipboardPath(input, "windows")
	want := `C:\Users\me\project\sub\file.txt`
	if got != want {
		t.Fatalf("normalizeClipboardPath(%q, windows) = %q, want %q", input, got, want)
	}
}

func TestNormalizeClipboardPathUnix(t *testing.T) {
	input := "/tmp/project/dir/../file.txt"
	got := normalizeClipboardPath(input, "linux")
	want := "/tmp/project/file.txt"
	if got != want {
		t.Fatalf("normalizeClipboardPath(%q, linux) = %q, want %q", input, got, want)
	}
}
