package render

import (
	"slices"
	"strings"
	"testing"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestBuildFooterHelpSegments_DefaultMode(t *testing.T) {
	state := &statepkg.AppState{
		HideHiddenFiles:    true,
		EditorAvailable:    true,
		ClipboardAvailable: true,
	}

	got := buildFooterHelpSegments(state)
	want := []string{
		"↑/↓/↵/→/←: navigate",
		"[]: history",
		"~: home",
		"/: filter",
		"f: search",
		"r: refresh",
		"→: preview full",
		"P: open pager",
		".: toggle hidden",
		"y: yank path",
		"e: edit file",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("default help mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildFooterHelpSegments_FilterMode(t *testing.T) {
	state := &statepkg.AppState{
		FilterActive: true,
	}

	got := buildFooterHelpSegments(state)
	wantPrefix := []string{
		"type: filter",
		"Esc: exit filter",
		"↵: accept selection",
		"←: clear query",
	}

	if len(got) != len(wantPrefix) {
		t.Fatalf("filter help should only include contextual hints, got: %v", got)
	}

	if !slices.Equal(got, wantPrefix) {
		t.Fatalf("filter help mismatch\nwant: %#v\n got: %#v", wantPrefix, got)
	}
}

func TestBuildFooterHelpSegments_GlobalSearchMode(t *testing.T) {
	state := &statepkg.AppState{
		GlobalSearchActive: true,
	}

	got := buildFooterHelpSegments(state)

	wantPrefix := []string{
		"type: search",
		"↵: navigate to",
		"Esc: clear/exit",
		"↑↓: select match",
		"PgUp/PgDn: page",
	}

	if !slices.Equal(got, wantPrefix) {
		t.Fatalf("global search help mismatch\nwant: %#v\n got: %#v", wantPrefix, got)
	}
}

func TestBuildFooterHelpTextPadding(t *testing.T) {
	state := &statepkg.AppState{}

	text := buildFooterHelpText(state)
	if !strings.HasPrefix(text, " ") || !strings.HasSuffix(text, " ") {
		t.Fatalf("help text missing padding: %q", text)
	}
}

func TestBuildFooterHelpSegments_PreviewMode(t *testing.T) {
	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}

	got := buildFooterHelpSegments(state)
	wantFocus := []string{
		"Esc/←/q: exit",
		"↑↓/Pg: scroll",
		"Home/End: jump",
		"w: toggle wrap",
		"P: open pager",
	}

	if len(got) < len(wantFocus) {
		t.Fatalf("expected preview focus help, got %v", got)
	}

	for i, want := range wantFocus {
		if got[i] != want {
			t.Fatalf("preview focus mismatch at %d: want %q got %q", i, want, got[i])
		}
	}

	for _, segment := range got {
		if segment == ".: toggle hidden" {
			t.Fatal("fullscreen help should not include toggle hidden shortcut")
		}
	}
}
