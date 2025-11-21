//go:build !windows

package app

func flushConsoleInput() error {
	return nil
}
