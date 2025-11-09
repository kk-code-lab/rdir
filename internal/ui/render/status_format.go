package render

import (
	"fmt"
	"strings"
	"time"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func formatSearchHeaderStatus(state *statepkg.AppState, status statepkg.IndexTelemetry) string {
	var parts []string
	if label := state.SearchStatusLabel(); label != "" {
		parts = append(parts, label)
	} else if state.GlobalSearchInProgress {
		parts = append(parts, "searching…")
	}
	if summary := formatIndexHeaderSummary(status); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, " · ")
}

func formatIndexHeaderSummary(status statepkg.IndexTelemetry) string {
	if status.RootPath == "" {
		return ""
	}
	if status.Building {
		return formatBuildingSummary(status, "index")
	}
	if status.Ready {
		return formatReadySummary(status, "index ready")
	}
	if status.Disabled {
		return formatDisabledIndexSummary(status)
	}
	return ""
}

func formatIndexStatusLine(status statepkg.IndexTelemetry) string {
	if status.RootPath == "" {
		return ""
	}
	switch {
	case status.Building:
		return formatBuildingSummary(status, "building index")
	case status.Ready:
		return formatReadySummary(status, "index ready")
	case status.Disabled:
		return formatDisabledIndexSummary(status)
	default:
		return ""
	}
}

func formatBuildingSummary(status statepkg.IndexTelemetry, prefix string) string {
	parts := []string{prefix}
	if status.FilesIndexed > 0 {
		parts = append(parts, fmt.Sprintf("%s files", formatCompactNumber(status.FilesIndexed)))
	}
	if !status.StartedAt.IsZero() {
		duration := time.Since(status.StartedAt)
		parts = append(parts, formatDurationShort(duration))
	}
	return strings.Join(parts, " · ")
}

func formatReadySummary(status statepkg.IndexTelemetry, prefix string) string {
	parts := []string{prefix}
	if status.FilesIndexed > 0 {
		parts = append(parts, fmt.Sprintf("%s files", formatCompactNumber(status.FilesIndexed)))
	}
	if status.Duration > 0 {
		parts = append(parts, formatDurationShort(status.Duration))
	}
	return strings.Join(parts, " · ")
}

func formatDisabledIndexSummary(status statepkg.IndexTelemetry) string {
	if status.LastError != "" {
		return "index disabled · " + status.LastError
	}
	return "index disabled"
}

func formatCompactNumber(n int) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000.0)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000.0)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000.0)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func trimTrailingZero(s string) string {
	return strings.TrimSuffix(strings.TrimSuffix(s, "0"), ".")
}

func formatDurationShort(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return trimTrailingZero(fmt.Sprintf("%.1fs", d.Seconds()))
	case d < time.Hour:
		return trimTrailingZero(fmt.Sprintf("%.1fm", d.Minutes()))
	default:
		return trimTrailingZero(fmt.Sprintf("%.1fh", d.Hours()))
	}
}
