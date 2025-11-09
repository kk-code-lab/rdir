package main

import (
	"fmt"
	"os"
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

	// Output the current directory so shell can cd to it
	if path := app.GetCurrentPath(); path != "" {
		fmt.Println(path)
	}
}
