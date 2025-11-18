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

    command %s &
    rdir_pid=$!
    wait $rdir_pid

    result_file="$TMPDIR/rdir_result_$rdir_pid.txt"
    if [ -f "$result_file" ] && [ ! -L "$result_file" ] && [ -O "$result_file" ]; then
        dest=$(cat "$result_file" 2>/dev/null)
        rm -f "$result_file"
        if [ -d "$dest" ] 2>/dev/null; then
            cd "$dest"
        fi
    else
        rm -f "$result_file" 2>/dev/null
    fi
}
`, quoted, quoted)
	case "fish":
		fmt.Printf(`function rdir
    if test (count $argv) -gt 0
        command %s $argv
        return $status
    end

    command %s &
    set rdir_pid $last_pid
    wait $rdir_pid

    set result_file "$TMPDIR/rdir_result_$rdir_pid.txt"
    if test -f "$result_file" -a ! -L "$result_file" -a -O "$result_file"
        set dest (cat "$result_file" 2>/dev/null)
        if test -d "$dest" 2>/dev/null
            builtin cd "$dest"
        end
    end
    rm -f "$result_file" 2>/dev/null
end
`, quoted, quoted)
	case "pwsh", "powershell":
		fmt.Printf(`function rdir {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    if ($Args.Count -gt 0) {
        & %s @Args
        return
    }

    $process = Start-Process -FilePath %s -NoNewWindow -PassThru
    $process.WaitForExit()

    $resultFile = Join-Path $env:TEMP "rdir_result_$($process.Id).txt"
    try {
        if (Test-Path $resultFile -PathType Leaf) {
            $dest = Get-Content $resultFile -Raw -ErrorAction SilentlyContinue | ForEach-Object { $_.Trim() }
            if ((Test-Path $dest -PathType Container) -and -not [string]::IsNullOrEmpty($dest)) {
                Set-Location $dest
            }
        }
    } finally {
        Remove-Item $resultFile -ErrorAction SilentlyContinue
    }
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

    command %s &
    rdir_pid=$!
    wait $rdir_pid

    result_file="$TMPDIR/rdir_result_$rdir_pid.txt"
    if [ -f "$result_file" ] && [ ! -L "$result_file" ] && [ -O "$result_file" ]; then
        dest=$(cat "$result_file" 2>/dev/null)
        rm -f "$result_file"
        if [ -d "$dest" ] 2>/dev/null; then
            cd "$dest"
        fi
    else
        rm -f "$result_file" 2>/dev/null
    fi
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
