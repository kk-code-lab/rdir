package textutil

import (
	"strings"
	"testing"
)

func TestSanitizeTerminalTextLeavesSafeInput(t *testing.T) {
	input := "safe-file.txt"
	if got := SanitizeTerminalText(input); got != input {
		t.Fatalf("expected %q to remain untouched, got %q", input, got)
	}
}

func TestSanitizeTerminalTextReplacesControlSequences(t *testing.T) {
	input := "bad\x1b[31m\npath"
	got := SanitizeTerminalText(input)
	if got != "bad?[31m path" {
		t.Fatalf("expected sanitized string \"bad?[31m path\", got %q", got)
	}
	if containsControl(got) {
		t.Fatalf("sanitized text should not contain control characters: %q", got)
	}
}

func TestSanitizeTerminalTextReplacesFormattingRunes(t *testing.T) {
	input := "a" + string(rune(0x202E)) + "b" + string(rune(0x200B)) + "c" + string(rune(0x00AD))
	got := SanitizeTerminalText(input)
	if containsRune(got, 0x202E) || containsRune(got, 0x200B) {
		t.Fatalf("sanitize left formatting runes in output: %q", got)
	}
	if !strings.Contains(got, "⟪RLO⟫") || !strings.Contains(got, "⟪ZWSP⟫") || !strings.Contains(got, "⟪SHY⟫") {
		t.Fatalf("expected formatting runes to be labeled, got %q", got)
	}
}

func TestReplaceFormattingRunes(t *testing.T) {
	input := "x" + string(rune(0x061C)) + "y"
	want := "x⟪ALM⟫y"
	if got, ok := ReplaceFormattingRunes(input); !ok || got != want {
		t.Fatalf("ReplaceFormattingRunes = (%q,%v), want (%q,true)", got, ok, want)
	}
}

func TestHasFormattingRunes(t *testing.T) {
	if HasFormattingRunes("plain") {
		t.Fatalf("expected plain text to have no formatting runes")
	}
	with := string(rune(0x2067))
	if !HasFormattingRunes("hi" + with) {
		t.Fatalf("expected formatting runes to be detected")
	}
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func containsRune(s string, target rune) bool {
	for _, r := range s {
		if r == target {
			return true
		}
	}
	return false
}
