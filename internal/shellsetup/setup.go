package shellsetup

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
)

type ParentShellFunc func() string

type Config struct {
	DetectParent ParentShellFunc
}

func PrintSetup(shellOverride string, cfg Config) {
	parent := cfg.DetectParent
	if parent == nil {
		parent = DetectParentShellName
	}

	shell := normalizeShellName(shellOverride)
	if shell == "" {
		shell = detectShell(parent)
	}
	shell = canonicalShellName(shell)

	rpath, err := os.Executable()
	if err != nil {
		rpath = "rdir"
	}
	quoted := strconv.Quote(rpath)

	switch shell {
	case "bash", "zsh", "sh", "ksh":
		fmt.Printf(`rdir() {
    if [ "$#" -gt 0 ]; then
        command %s "$@"
        return $?
    fi

    dest="$(command %s)"
    status=$?
    if [ $status -ne 0 ] || [ -z "$dest" ]; then
        dest="$PWD"
    fi
    cd "$dest"
}
`, quoted, quoted)
	case "fish":
		fmt.Printf(`function rdir
    if test (count $argv) -gt 0
        command %s $argv
        return $status
    end

    set dest (command %s)
    if test $status -ne 0 -o -z "$dest"
        set dest $PWD
    end
    builtin cd "$dest"
end
`, quoted, quoted)
	case "pwsh", "powershell":
		fmt.Printf(`function rdir {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    if ($Args.Count -gt 0) {
        & %s @Args
        return
    }

    $dest = & %s
    if (-not $dest) {
        $dest = (Get-Location).Path
    }
    Set-Location $dest
}
`, quoted, quoted)
	case "tcsh", "csh":
		fmt.Printf("alias rdir 'cd `%s`'\n", rpath)
	case "cmd":
		fmt.Printf(`:: Save as rdir.cmd and run "call rdir.cmd" from cmd.exe sessions.
@echo off
if "%%~1"==""
(
    for /f "delims=" %%%%d in ('%s') do (
        if not "%%%%d"=="" cd /d "%%%%d"
    )
    exit /b 0
) else (
    %s %%*
    exit /b %%errorlevel%%
)
`, quoted, quoted)
	default:
		fmt.Printf(`rdir() {
    if [ "$#" -gt 0 ]; then
        command %s "$@"
        return $?
    fi

    dest="$(command %s)"
    status=$?
    if [ $status -ne 0 ] || [ -z "$dest" ]; then
        dest="$PWD"
    fi
    cd "$dest"
}
`, quoted, quoted)
	}
}

func detectShell(parent ParentShellFunc) string {
	return detectShellInternal(runtime.GOOS, os.Getenv, parent)
}

func detectShellInternal(goos string, getenv func(string) string, parent ParentShellFunc) string {
	if shell := canonicalShellName(normalizeShellName(getenv("SHELL"))); shell != "" {
		return shell
	}

	if parent != nil {
		if shell := canonicalShellName(normalizeShellName(parent())); shell != "" {
			return shell
		}
	}

	if strings.EqualFold(goos, "windows") {
		if shell := canonicalShellName(normalizeShellName(getenv("COMSPEC"))); shell != "" {
			switch shell {
			case "pwsh", "cmd":
				return shell
			}
		}
		return "pwsh"
	}

	return "bash"
}

func canonicalShellName(name string) string {
	switch name {
	case "powershell":
		return "pwsh"
	default:
		return name
	}
}

func normalizeShellName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = extractExecutable(value)
	if value == "" {
		return ""
	}

	value = strings.Trim(value, `"'`)
	value = strings.ReplaceAll(value, "\\", "/")
	base := path.Base(value)
	base = strings.ToLower(base)
	base = strings.TrimSuffix(base, ".exe")
	return strings.TrimSpace(base)
}

func extractExecutable(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if strings.HasPrefix(value, "\"") {
		value = value[1:]
		if idx := strings.IndexRune(value, '"'); idx >= 0 {
			return value[:idx]
		}
		return value
	}

	if strings.HasPrefix(value, "'") {
		value = value[1:]
		if idx := strings.IndexRune(value, '\''); idx >= 0 {
			return value[:idx]
		}
		return value
	}

	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return value[:idx]
	}

	return value
}
