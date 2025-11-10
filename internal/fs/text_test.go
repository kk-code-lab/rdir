package fs

import "testing"

func TestIsTextFileDetectsUTF16LE(t *testing.T) {
	content := []byte{0xFF, 0xFE, 0x41, 0x00, 0x0D, 0x00, 0x0A, 0x00}
	if !IsTextFile("config.ini", content) {
		t.Fatalf("expected UTF-16 LE content to be treated as text")
	}
}

func TestNormalizeTextContentUTF16LE(t *testing.T) {
	content := []byte{0xFF, 0xFE, 0x41, 0x00, 0x0D, 0x00, 0x0A, 0x00}
	got := NormalizeTextContent(content)
	want := "A\r\n"
	if got != want {
		t.Fatalf("NormalizeTextContent returned %q, want %q", got, want)
	}
}
