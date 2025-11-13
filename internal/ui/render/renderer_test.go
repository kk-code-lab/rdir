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
	if wide.previewWidth < narrow.previewWidth {
		t.Fatalf("expected wider terminal to produce >= preview width (narrow=%d, wide=%d)", narrow.previewWidth, wide.previewWidth)
	}
}

func TestPreviewLayoutUsesFixedRatio(t *testing.T) {
	r := NewRenderer(nil)
	state := &statepkg.AppState{}

	layout := r.computeLayout(200, state)
	if !layout.showPreview {
		t.Fatalf("expected preview to be visible for ratio check")
	}

	total := layout.mainPanelWidth + layout.previewWidth
	expectedPreview := clampPreviewRatioWidth(total)
	if diff := absInt(layout.previewWidth - expectedPreview); diff > 1 {
		t.Fatalf("expected preview near %d cols (+/-1), got %d (diff=%d)", expectedPreview, layout.previewWidth, diff)
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

	layout := r.computeLayout(120, state)
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

func TestBinaryPreviewHiddenWhenTooNarrow(t *testing.T) {
	r := NewRenderer(nil)
	state := binaryPreviewState()

	layout := r.computeLayout(90, state)
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

func TestComputeHighlightSpansDisjointMatches(t *testing.T) {
	spans := computeHighlightSpans("cch", "cache_handler", false)
	want := []highlightSpan{
		{start: 0, end: 1},
		{start: 2, end: 4},
	}
	if len(spans) != len(want) {
		t.Fatalf("expected %d spans, got %d (%v)", len(want), len(spans), spans)
	}
	for i := range spans {
		if spans[i] != want[i] {
			t.Fatalf("span %d mismatch: got %+v want %+v", i, spans[i], want[i])
		}
	}
}

func TestPreviewWidthShrinksToTextContent(t *testing.T) {
	r := NewRenderer(nil)
	state := textPreviewState("short text", "another line")

	layout := r.computeLayout(220, state)
	if !layout.showPreview {
		t.Fatalf("expected preview visible")
	}

	textWidth := r.estimateTextPreviewWidth(state.PreviewData) + previewInnerPadding*2
	expected := maxInt(textWidth, minPreviewPanelWidth)
	if layout.previewWidth != expected {
		t.Fatalf("preview width should align with text (expected %d, got %d)", expected, layout.previewWidth)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clampPreviewRatioWidth(total int) int {
	width := int(float64(total)*previewPanelRatio + 0.5)
	if width < minPreviewPanelWidth {
		width = minPreviewPanelWidth
	}
	if width > previewWidthCap {
		width = previewWidthCap
	}
	return width
}

func textPreviewState(lines ...string) *statepkg.AppState {
	return &statepkg.AppState{
		PreviewData: &statepkg.PreviewData{
			TextLines: lines,
		},
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestComputeHighlightSpansRespectsCaseSensitivity(t *testing.T) {
	insensitive := computeHighlightSpans("AbC", "Abc", false)
	if len(insensitive) == 0 {
		t.Fatalf("expected insensitive search to highlight")
	}

	sensitive := computeHighlightSpans("AbC", "Abc", true)
	if len(sensitive) != 1 || sensitive[0].start != 0 || sensitive[0].end != 2 {
		t.Fatalf("expected only exact-case prefix to match in case-sensitive mode, got %+v", sensitive)
	}
}

func TestConvertMatchSpansToHighlightsClampsAndMerges(t *testing.T) {
	spans := []statepkg.MatchSpan{
		{Start: -2, End: 0},
		{Start: 2, End: 4},
		{Start: 3, End: 5},
	}
	out := convertMatchSpansToHighlights(spans, "abcdef")
	if len(out) != 2 {
		t.Fatalf("expected 2 merged spans, got %v", out)
	}
	if out[0].start != 0 || out[0].end != 1 {
		t.Fatalf("unexpected first span %+v", out[0])
	}
	if out[1].start != 2 || out[1].end != 6 {
		t.Fatalf("unexpected second span %+v", out[1])
	}
}
