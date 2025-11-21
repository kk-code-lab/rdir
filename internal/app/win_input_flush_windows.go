//go:build windows

package app

import "golang.org/x/sys/windows"

func flushConsoleInput() error {
	handle, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return err
	}
	return windows.FlushConsoleInputBuffer(handle)
}
