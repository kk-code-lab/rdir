//go:build windows

package app

import "os"

func contSignals() []os.Signal {
	return nil
}
