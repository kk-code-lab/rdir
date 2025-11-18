package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	apppkg "github.com/kk-code-lab/rdir/internal/app"
	"github.com/kk-code-lab/rdir/internal/shellsetup"
)

func printHelp() {
	fmt.Print(`rdir - Terminal-based file manager

USAGE:
    rdir [OPTIONS]

OPTIONS:
    -h, --help            Show this help message and exit
    -s, --setup [SHELL]   Output shell integration snippet (optionally force SHELL)
`)
}

var parentShellDetector = shellsetup.DetectParentShellName

func main() {
	// Set UTF-8 as fallback encoding for maximum compatibility
	// This ensures Polish and other Unicode characters display correctly
	tcell.SetEncodingFallback(tcell.EncodingFallbackUTF8)

	// Parse command-line arguments
	if len(os.Args) > 1 {
		arg := os.Args[1]
		switch {
		case arg == "-h" || arg == "--help":
			printHelp()
			os.Exit(0)
		case arg == "-s" || arg == "--setup":
			shellOverride := ""
			if len(os.Args) > 2 {
				shellOverride = os.Args[2]
			}
			shellsetup.PrintSetup(shellOverride, shellsetup.Config{DetectParent: parentShellDetector})
			os.Exit(0)
		case strings.HasPrefix(arg, "--setup="):
			shellOverride := strings.TrimPrefix(arg, "--setup=")
			shellsetup.PrintSetup(shellOverride, shellsetup.Config{DetectParent: parentShellDetector})
			os.Exit(0)
		}
	}

	app, err := apppkg.NewApplication()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = app.Close()
	}()

	app.Run()

	// Write selected directory to temp file for shell integration
	// Use PID to make filename unique (supports multiple rdir instances)
	if path := app.GetCurrentPath(); path != "" {
		tempDir := os.TempDir()
		resultFile := filepath.Join(tempDir, fmt.Sprintf("rdir_result_%d.txt", os.Getpid()))
		// Write with 0600 permissions (owner only) for security
		if err := os.WriteFile(resultFile, []byte(path), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not write result file: %v\n", err)
		}
	}
}
