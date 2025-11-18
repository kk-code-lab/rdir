package app

import (
	"path/filepath"
	"testing"
)

func TestBuildBreadcrumbPath(t *testing.T) {
	tests := []struct {
		name     string
		segments []string
		idx      int
		expect   string
	}{
		{
			name:     "windows drive root",
			segments: []string{"C:"},
			idx:      0,
			expect:   "C:" + string(filepath.Separator),
		},
		{
			name:     "windows drive nested",
			segments: []string{"C:", "Users", "me"},
			idx:      2,
			expect:   filepath.Join("C:"+string(filepath.Separator), "Users", "me"),
		},
		{
			name:     "posix root",
			segments: []string{"/", "home", "me"},
			idx:      2,
			expect:   filepath.Join(string(filepath.Separator), "home", "me"),
		},
	}

	for _, tt := range tests {
		if got := buildBreadcrumbPath(tt.segments, tt.idx); got != tt.expect {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.expect, got)
		}
	}
}
