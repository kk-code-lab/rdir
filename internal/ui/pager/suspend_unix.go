//go:build !windows

package pager

import (
	"syscall"

	"golang.org/x/term"
)

func (p *PreviewPager) suspendToShell() error {
	if p.input != nil && p.restoreTerm != nil {
		_ = term.Restore(int(p.input.Fd()), p.restoreTerm)
	}
	if p.writer != nil {
		p.writeString("\x1b[?25h")
		p.writeString("\x1b[?7h")
		_ = p.writer.Flush()
	} else {
		p.writeString("\x1b[?25h")
		p.writeString("\x1b[?7h")
	}
	return syscall.Kill(0, syscall.SIGTSTP)
}
