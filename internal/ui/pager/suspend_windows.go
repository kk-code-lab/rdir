//go:build windows

package pager

// On Windows, SIGTSTP is unavailable; ignore Ctrl+Z in pager.
func (p *PreviewPager) suspendToShell() error {
	return nil
}
