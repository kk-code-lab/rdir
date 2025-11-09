package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

var pagerLookPath = exec.LookPath

func detectClipboard() ([]string, bool) {
	return detectClipboardInternal(runtime.GOOS, exec.LookPath)
}

func detectClipboardInternal(goos string, lookPath func(string) (string, error)) ([]string, bool) {
	trySingle := func(candidates ...string) ([]string, bool) {
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if path, err := lookPath(candidate); err == nil && path != "" {
				return []string{path}, true
			}
		}
		return nil, false
	}

	if strings.EqualFold(goos, "windows") {
		if cmd, ok := trySingle("clip.exe", "clip"); ok {
			return cmd, true
		}
		for _, ps := range []string{"powershell", "powershell.exe", "pwsh"} {
			if path, err := lookPath(ps); err == nil && path != "" {
				return []string{path, "-NoLogo", "-NoProfile", "-Command", "Set-Clipboard"}, true
			}
		}
	}

	commands := []string{"pbcopy", "xclip", "wl-copy", "xsel"}
	for _, cmd := range commands {
		if resolved, err := lookPath(cmd); err == nil && resolved != "" {
			return []string{resolved}, true
		}
	}

	return nil, false
}

func detectEditorCommand() ([]string, bool) {
	return detectEditorCommandInternal(runtime.GOOS, os.Getenv, exec.LookPath)
}

func detectEditorCommandInternal(goos string, getenv func(string) string, lookPath func(string) (string, error)) ([]string, bool) {
	candidates := []string{getenv("VISUAL"), getenv("EDITOR")}

	for _, candidate := range candidates {
		args := parseEditorCommand(candidate)
		if len(args) == 0 {
			continue
		}
		if resolved, ok := resolveEditorExecutableWithLookup(args[0], lookPath); ok {
			args[0] = resolved
			return args, true
		}
	}

	var defaults [][]string
	if strings.EqualFold(goos, "windows") {
		defaults = [][]string{
			{"code", "--wait"},
			{"notepad++.exe"},
			{"notepad.exe"},
		}
	} else {
		defaults = [][]string{
			{"vim"},
			{"nano"},
		}
	}

	for _, def := range defaults {
		if len(def) == 0 {
			continue
		}
		if resolved, ok := resolveEditorExecutableWithLookup(def[0], lookPath); ok {
			args := append([]string{resolved}, def[1:]...)
			return args, true
		}
	}

	return nil, false
}

func parseEditorCommand(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for _, r := range cmd {
		switch r {
		case '\'':
			if inDouble {
				current.WriteRune(r)
			} else {
				inSingle = !inSingle
			}
			continue
		case '"':
			if inSingle {
				current.WriteRune(r)
			} else {
				inDouble = !inDouble
			}
			continue
		default:
			if !inSingle && !inDouble && unicode.IsSpace(r) {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
				continue
			}
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if len(args) > 0 {
		args[0] = expandUserPath(args[0])
	}

	return args
}

func expandUserPath(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}

	if len(path) == 1 {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}

	sep := path[1]
	if sep != '/' && sep != '\\' {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, path[2:])
}

func resolveEditorExecutableWithLookup(cmd string, lookPath func(string) (string, error)) (string, bool) {
	if cmd == "" {
		return "", false
	}

	if expanded := expandUserPath(cmd); expanded != cmd {
		cmd = expanded
	}

	path, err := lookPath(cmd)
	if err != nil {
		return "", false
	}
	return path, true
}

func detectPagerCommand(goos string, pagerEnv string, lookPath func(string) (string, error)) []string {
	pagerEnv = strings.TrimSpace(pagerEnv)
	if pagerEnv != "" {
		if args := parseEditorCommand(pagerEnv); len(args) > 0 {
			return args
		}
	}
	return defaultPagerCommand(goos, lookPath)
}

func defaultPagerCommand(goos string, lookPath func(string) (string, error)) []string {
	if strings.EqualFold(goos, "windows") {
		candidates := []string{"more.com", "more"}
		if lookPath != nil {
			for _, candidate := range candidates {
				if path, err := lookPath(candidate); err == nil && path != "" {
					return []string{path}
				}
			}
		}
		return []string{"cmd", "/C", "type"}
	}
	return []string{"less"}
}
