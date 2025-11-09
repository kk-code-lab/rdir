package shellsetup

import (
	"testing"
)

func TestDetectShellInternal(t *testing.T) {
	tests := []struct {
		name          string
		goos          string
		envShell      string
		envComspec    string
		parent        func() string
		expectedShell string
	}{
		{
			name:          "uses SHELL when set",
			goos:          "linux",
			envShell:      "/bin/zsh",
			expectedShell: "zsh",
		},
		{
			name:          "falls back to parent shell",
			goos:          "linux",
			parent:        func() string { return "/usr/bin/bash" },
			expectedShell: "bash",
		},
		{
			name:          "windows prefers COMSPEC",
			goos:          "windows",
			envComspec:    `C:\Windows\System32\cmd.exe`,
			expectedShell: "cmd",
		},
		{
			name:          "windows fallback",
			goos:          "windows",
			expectedShell: "pwsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := func(key string) string {
				switch key {
				case "SHELL":
					return tt.envShell
				case "COMSPEC":
					return tt.envComspec
				default:
					return ""
				}
			}
			got := detectShellInternal(tt.goos, env, tt.parent)
			if got != tt.expectedShell {
				t.Fatalf("detectShellInternal() = %q, want %q", got, tt.expectedShell)
			}
		})
	}
}
