//go:build !windows && !plan9 && !js && !wasip1

package pager

import (
	"os"
	"syscall"
)

func resizeSignals() []os.Signal {
	return []os.Signal{syscall.SIGWINCH}
}
