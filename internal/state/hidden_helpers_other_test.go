//go:build !windows

package state

func markHiddenForTest(string) error {
	return nil
}
