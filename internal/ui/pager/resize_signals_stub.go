//go:build windows || plan9 || js || wasip1

package pager

import "os"

func resizeSignals() []os.Signal {
	return nil
}
