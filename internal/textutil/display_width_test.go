package textutil

import "testing"

func TestDisplayWidthGraphemeClusters(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"warning emoji with VS16", "⚠️", 2},
		{"thumbs up with skin tone", "👍🏻", 2},
		{"family zwj", "👨‍👩‍👧", 2},
		{"flag regional indicators", "🇵🇱", 2},
		{"keycap one", "1️⃣", 2},
		{"mixed ascii + emoji", "a⚠️b", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayWidth(tt.text); got != tt.want {
				t.Fatalf("DisplayWidth(%q)=%d want %d", tt.text, got, tt.want)
			}
		})
	}
}
