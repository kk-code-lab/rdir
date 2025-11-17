//go:build windows || plan9 || js || wasip1

package pager

func (p *PreviewPager) startKeyReader(done <-chan struct{}) (<-chan keyEvent, <-chan error, func()) {
	return nil, nil, nil
}
