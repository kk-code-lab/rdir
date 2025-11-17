//go:build !windows

package app

import (
	"os"
	"syscall"
)

func contSignals() []os.Signal {
	return []os.Signal{syscall.SIGCONT}
}
