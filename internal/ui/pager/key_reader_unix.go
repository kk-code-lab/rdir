//go:build !windows && !plan9 && !js && !wasip1

package pager

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func (p *PreviewPager) startKeyReader(done <-chan struct{}) (<-chan keyEvent, <-chan error, func()) {
	events := make(chan keyEvent, 1)
	errCh := make(chan error, 1)
	if p.input == nil {
		errCh <- errors.New("no pager input available")
		return events, errCh, nil
	}
	cancelR, cancelW, err := os.Pipe()
	if err != nil {
		errCh <- err
		return events, errCh, nil
	}
	stop := func() {
		_, _ = cancelW.Write([]byte{1})
		_ = cancelW.Close()
	}

	go func() {
		defer func() {
			_ = cancelR.Close()
		}()
		inputFd := int(p.input.Fd())
		cancelFd := int(cancelR.Fd())
		for {
			var readfds unix.FdSet
			fdSetAdd(&readfds, inputFd)
			fdSetAdd(&readfds, cancelFd)
			maxfd := inputFd
			if cancelFd > maxfd {
				maxfd = cancelFd
			}
			n, err := unix.Select(maxfd+1, &readfds, nil, nil, nil)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			if n == 0 {
				continue
			}
			if fdSetHas(&readfds, cancelFd) {
				return
			}
			if fdSetHas(&readfds, inputFd) {
				ev, err := p.readKeyEvent()
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				select {
				case <-done:
					return
				case events <- ev:
				}
			}
		}
	}()

	go func() {
		<-done
		_, _ = cancelW.Write([]byte{1})
		_ = cancelW.Close()
	}()

	return events, errCh, stop
}

func fdSetAdd(set *unix.FdSet, fd int) {
	if fd < 0 {
		return
	}
	set.Bits[fd/64] |= 1 << (uint(fd) % 64)
}

func fdSetHas(set *unix.FdSet, fd int) bool {
	if fd < 0 {
		return false
	}
	return set.Bits[fd/64]&(1<<(uint(fd)%64)) != 0
}
