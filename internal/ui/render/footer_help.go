package render

import (
	"fmt"
	"strings"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

// buildFooterHelpText returns the contextual footer hint string with leading/trailing padding.
func buildFooterHelpText(state *statepkg.AppState) string {
	parts := buildFooterHelpSegments(state)
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, "  ") + " "
}

// buildFooterHelpSegments assembles context-aware help hints for the footer.
func buildFooterHelpSegments(state *statepkg.AppState) []string {
	if state == nil {
		return nil
	}

	segments := contextualHelpSegments(state)
	segments = append(segments, persistentHelpSegments(state)...)

	return segments
}

func contextualHelpSegments(state *statepkg.AppState) []string {
	switch {
	case state.GlobalSearchActive:
		return []string{
			"type: search",
			"↵: navigate to",
			"Esc: clear/exit",
			"↑↓: select match",
			"PgUp/PgDn: page",
		}
	case state.FilterActive:
		return []string{
			"type: filter",
			"Esc: exit filter",
			"↵: accept selection",
			"←: clear query",
		}
	case state.PreviewFullScreen:
		return []string{
			"Esc/←/q: exit",
			"↑↓/Pg: scroll",
			"Home/End: jump",
			"w: toggle wrap",
			"P: open pager",
		}
	default:
		return []string{
			"↑/↓/↵/→/←: navigate",
			"[]: history",
			"~: home",
			"/: filter",
			"f: search",
			"r: refresh",
			"→: preview full",
			"P: open pager",
		}
	}
}

func persistentHelpSegments(state *statepkg.AppState) []string {
	if state == nil {
		return nil
	}

	if state.FilterActive || state.GlobalSearchActive {
		return nil
	}

	hiddenStatus := "visible"
	if state.HideHiddenFiles {
		hiddenStatus = "hidden"
	}

	segments := []string{}
	if !state.PreviewFullScreen {
		segments = append(segments, fmt.Sprintf(".: toggle %s", hiddenStatus))
	}

	if state.ClipboardAvailable {
		segments = append(segments, "y: yank path")
	}

	if state.EditorAvailable {
		segments = append(segments, "e: edit file")
	}

	return segments
}
