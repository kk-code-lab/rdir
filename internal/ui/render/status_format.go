package render

import (
	"fmt"
	"math"
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
	if files := formatFilesProgress(status, true); files != "" {
		parts = append(parts, files)
	}
	if rate := formatFilesPerSecond(status); rate != "" {
		parts = append(parts, rate)
	}
	if !status.StartedAt.IsZero() {
		duration := time.Since(status.StartedAt)
		parts = append(parts, formatDurationShort(duration))
	}
	return strings.Join(parts, " · ")
}

func formatReadySummary(status statepkg.IndexTelemetry, prefix string) string {
	parts := []string{prefix}
	if files := formatFilesProgress(status, false); files != "" {
		parts = append(parts, files)
	}
	if rate := formatFilesPerSecond(status); rate != "" {
		parts = append(parts, rate)
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

func formatFilesProgress(status statepkg.IndexTelemetry, building bool) string {
	if status.TotalFiles > 0 {
		if building && status.FilesIndexed > 0 && status.FilesIndexed < status.TotalFiles {
			percent := int(math.Round(float64(status.FilesIndexed) / float64(status.TotalFiles) * 100.0))
			return fmt.Sprintf("%s/%s (%d%%)", formatCompactNumber(status.FilesIndexed), formatCompactNumber(status.TotalFiles), percent)
		}
		return fmt.Sprintf("%s files", formatCompactNumber(status.TotalFiles))
	}
	if status.FilesIndexed > 0 {
		return fmt.Sprintf("%s files", formatCompactNumber(status.FilesIndexed))
	}
	return ""
}

func formatFilesPerSecond(status statepkg.IndexTelemetry) string {
	if status.FilesIndexed <= 0 {
		return ""
	}

	var duration time.Duration
	switch {
	case status.Building && !status.StartedAt.IsZero():
		duration = time.Since(status.StartedAt)
	case status.Duration > 0:
		duration = status.Duration
	case !status.StartedAt.IsZero():
		duration = time.Since(status.StartedAt)
	default:
		return ""
	}
	if duration <= 0 {
		return ""
	}

	rate := float64(status.FilesIndexed) / duration.Seconds()
	if rate <= 0 {
		return ""
	}
	return fmt.Sprintf("@ %s/s", formatRateValue(rate))
}

func formatRateValue(rate float64) string {
	switch {
	case rate >= 1_000_000_000:
		return trimTrailingZero(fmt.Sprintf("%.1fB", rate/1_000_000_000))
	case rate >= 1_000_000:
		return trimTrailingZero(fmt.Sprintf("%.1fM", rate/1_000_000))
	case rate >= 1_000:
		return trimTrailingZero(fmt.Sprintf("%.1fk", rate/1_000))
	case rate >= 10:
		return fmt.Sprintf("%.0f", rate)
	default:
		return trimTrailingZero(fmt.Sprintf("%.1f", rate))
	}
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
