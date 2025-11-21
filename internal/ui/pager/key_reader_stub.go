//go:build plan9 || js || wasip1 || (!windows && !unix && !darwin && !linux && !freebsd && !openbsd && !netbsd)

package pager

// startKeyReader is a no-op stub for unsupported platforms.
func (p *PreviewPager) startKeyReader(done <-chan struct{}) (<-chan keyEvent, <-chan error, func()) {
	return nil, nil, nil
}
