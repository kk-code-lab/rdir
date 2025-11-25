package render

import (
	"strings"
	"testing"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestBuildHelpOverlayLinesIncludesSections(t *testing.T) {
	state := &statepkg.AppState{HideHiddenFiles: true}

	lines := buildHelpOverlayLines(state)

	assertContains := func(substr string) {
		found := false
		for _, line := range lines {
			if strings.Contains(line, substr) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected lines to contain %q, got %v", substr, lines)
		}
	}

	assertContains("Navigation")
	assertContains("Filter & Search")
	assertContains("Preview & Pager")
	assertContains("Exit")
	assertContains("Show hidden files")
	assertContains("external pager")
	assertContains("external editor")
	assertContains("external pager")
}

func TestBuildHelpOverlayLinesReflectsHiddenToggle(t *testing.T) {
	state := &statepkg.AppState{HideHiddenFiles: false}
	lines := buildHelpOverlayLines(state)

	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "Hide hidden files") {
		t.Fatalf("expected help to show hide instruction when hidden files visible, got %v", lines)
	}
}
