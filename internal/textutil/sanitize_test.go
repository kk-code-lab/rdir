package textutil

import "testing"

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

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
