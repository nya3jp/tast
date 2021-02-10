// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// pipeWatcher asynchronously watches an FD corresponding to a pipe and reports when
// the read end of it is closed.
type pipeWatcher struct {
	readClosed chan struct{} // closed by goroutine when read end of writeFD is closed
	errCh      chan error    // written to by goroutine on completion to report error (or success)
	closer     *os.File      // read end of another pipe closed by close() to tell goroutine to exit
}

// newPipeWatcher returns a new pipeWatcher that watches the read end of writeFD for closure.
func newPipeWatcher(writeFD int) (*pipeWatcher, error) {
	// Create a pipe for communication with the goroutine.
	// The read end is closed in close() to tell the goroutine to exit.
	// The write end is monitored via epoll in the goroutine and later closed there.
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	pw := &pipeWatcher{
		readClosed: make(chan struct{}),
		errCh:      make(chan error, 1),
		closer:     r,
	}

	// Start a goroutine that uses epoll to watch for the read ends of writeFD and pw.closer being closed.
	go func() {
		defer w.Close()
		defer close(pw.errCh)

		pw.errCh <- func() error {
			epollFD, err := syscall.EpollCreate1(0)
			if err != nil {
				return fmt.Errorf("failed creating epoll FD: %v", err)
			}
			defer syscall.Close(epollFD)

			for _, fd := range []int{writeFD, int(w.Fd())} {
				if err := syscall.EpollCtl(epollFD, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Fd: int32(fd)}); err != nil {
					return fmt.Errorf("failed to add FD %d: %v", fd, err)
				}
			}

			// See epoll_ctl(2): "[EPOLLERR] is also reported for the write end of a pipe
			// when the read end has been closed. epoll_wait(2) will always report for this event;
			// it is not necessary to set it in _events_."
			events := make([]syscall.EpollEvent, 1)
			for {
				ret, err := syscall.EpollWait(epollFD, events, -1)
				if ret != -1 || err == nil {
					break
				}
				if !errors.Is(err, syscall.EINTR) {
					return fmt.Errorf("epoll_wait: %v", err)
				}
			}
			if ev := events[0]; ev.Fd == int32(writeFD) && ev.Events == syscall.EPOLLERR {
				// The read end of writeFD was closed.
				close(pw.readClosed)
				return nil
			} else if ev.Fd == int32(w.Fd()) && ev.Events == syscall.EPOLLERR {
				// The read end of w.Fd was closed (i.e. close() was called).
				return nil
			} else {
				return fmt.Errorf("epoll_wait reported unexpected event %+v", ev)
			}
		}()
	}()

	return pw, nil
}

// close must be called to release resources and stop watching the FD.
func (pw *pipeWatcher) close() error {
	pw.closer.Close()
	return <-pw.errCh
}
