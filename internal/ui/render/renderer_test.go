package render

import (
	"testing"

	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestTruncateTextToWidth(t *testing.T) {
	r := NewRenderer(nil)

	tests := []struct {
		name   string
		text   string
		width  int
		expect string
	}{
		{
			name:   "fits without truncation",
			text:   "file.txt",
			width:  20,
			expect: "file.txt",
		},
		{
			name:   "adds ellipsis when needed",
			text:   "verylongname",
			width:  6,
			expect: "veryl…",
		},
		{
			name:   "only ellipsis when width too small",
			text:   "example",
			width:  1,
			expect: "…",
		},
		{
			name:   "multi-byte characters respected",
			text:   "你好世界",
			width:  5,
			expect: "你好…",
		},
		{
			name:   "returns empty when width is zero",
			text:   "anything",
			width:  0,
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := r.truncateTextToWidth(tt.text, tt.width)
			if actual != tt.expect {
				t.Fatalf("expected %q, got %q (width %d)", tt.expect, actual, tt.width)
			}
		})
	}
}

func TestMeasureTextWidth(t *testing.T) {
	r := NewRenderer(nil)

	if got := r.measureTextWidth("abc"); got != 3 {
		t.Fatalf("expected ASCII width 3, got %d", got)
	}

	if got := r.measureTextWidth("你好"); got != 4 {
		t.Fatalf("expected wide rune width 4, got %d", got)
	}
}

func TestComputeLayoutShowsPreviewOnModerateScreen(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	layout := r.computeLayout(150, state)
	if !layout.showPreview {
		t.Fatalf("expected preview on moderate terminal width")
	}
	if layout.mainPanelWidth < minMainPanelWidth {
		t.Fatalf("expected main panel width >= %d, got %d", minMainPanelWidth, layout.mainPanelWidth)
	}
	if layout.previewStart <= layout.mainPanelStart {
		t.Fatalf("preview should start after main panel")
	}
	if layout.previewWidth < minPreviewPanelWidth {
		t.Fatalf("expected preview to be at least %d cols, got %d", minPreviewPanelWidth, layout.previewWidth)
	}
}

func TestPreviewWidthScalesWithTerminal(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	narrow := r.computeLayout(150, state)
	wide := r.computeLayout(220, state)
	if !narrow.showPreview || !wide.showPreview {
		t.Fatalf("expected preview visible for both terminal widths")
	}
	if wide.previewWidth <= narrow.previewWidth {
		t.Fatalf("expected wider terminal to produce larger preview (narrow=%d, wide=%d)", narrow.previewWidth, wide.previewWidth)
	}
}

func TestComputeLayoutHidesPreviewOnSmallScreens(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	layout := r.computeLayout(80, state)
	if layout.showPreview {
		t.Fatalf("preview should be hidden on narrow terminals")
	}
}

func TestComputeLayoutDropsSidebarWhenTooNarrow(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	layout := r.computeLayout(48, state)
	if layout.sidebarWidth != 0 {
		t.Fatalf("expected sidebar to be hidden, got width %d", layout.sidebarWidth)
	}
	if layout.mainPanelStart != 0 {
		t.Fatalf("main panel should start at column 0 when sidebar hidden")
	}
}

func TestBinaryPreviewUsesHexOnlyModeWhenTight(t *testing.T) {
	r := NewRenderer(nil)
	state := binaryPreviewState()

	layout := r.computeLayout(130, state)
	if !layout.showPreview {
		t.Fatalf("expected preview to be visible")
	}
	if layout.binaryMode != binaryPreviewModeHexOnly {
		t.Fatalf("expected hex-only mode, got %v (preview width %d)", layout.binaryMode, layout.previewWidth)
	}
	if layout.previewWidth < binaryHexPreviewMinWidth {
		t.Fatalf("hex-only width should be at least %d, got %d", binaryHexPreviewMinWidth, layout.previewWidth)
	}
}

func TestBinaryPreviewKeepsAsciiWhenWide(t *testing.T) {
	r := NewRenderer(nil)
	state := binaryPreviewState()

	layout := r.computeLayout(180, state)
	if layout.binaryMode != binaryPreviewModeFull {
		t.Fatalf("expected full binary mode, got %v (preview width %d)", layout.binaryMode, layout.previewWidth)
	}
	if layout.previewWidth < binaryFullPreviewMinWidth {
		t.Fatalf("full mode requires at least %d columns, got %d", binaryFullPreviewMinWidth, layout.previewWidth)
	}
}

func TestPreviewWidthNeverExceedsCap(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	layout := r.computeLayout(260, state)
	if layout.previewWidth > binaryFullPreviewMinWidth {
		t.Fatalf("preview width should be capped at %d, got %d", binaryFullPreviewMinWidth, layout.previewWidth)
	}
}

func TestBinaryPreviewHiddenWhenTooNarrow(t *testing.T) {
	r := NewRenderer(nil)
	state := binaryPreviewState()

	layout := r.computeLayout(110, state)
	if layout.showPreview {
		t.Fatalf("preview should hide when width insufficient (width=%d)", layout.previewWidth)
	}
}

func binaryPreviewState() *statepkg.AppState {
	return &statepkg.AppState{
		PreviewData: &statepkg.PreviewData{
			IsDir: false,
			BinaryInfo: statepkg.BinaryPreview{
				Lines: []string{
					"Binary preview (16 of 16 bytes)",
					"00000000  CF FA ED FE 0C 00 00 01  00 00 00 00 01 00 00 00  |................|",
				},
			},
		},
	}
}

func TestTextPreviewEstimationUsesTabs(t *testing.T) {
	r := NewRenderer(nil)
	preview := &statepkg.PreviewData{
		TextLines: []string{
			"\tshort",
			"\tfunc example() {",
			"\t\treturn 42",
			"\t}",
		},
	}

	width := r.desiredPreviewWidth(130, minPreviewPanelWidth, preview)
	expected := r.measureTextWidth(r.expandTabs(preview.TextLines[1], previewTabWidth))
	if width < expected {
		t.Fatalf("expected preview width >= %d to accommodate tabs, got %d", expected, width)
	}
}
