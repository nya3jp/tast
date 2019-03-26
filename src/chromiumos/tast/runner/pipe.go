// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"syscall"
)

// pipeWatcher asynchronously watches an FD corresponding to a pipe and reports when
// the read end of it is closed.
type pipeWatcher struct {
	epollFD    int           // used by epoll to monitor the pipe
	readClosed chan struct{} // channel that is closed when read end is closed
}

// newPipeWatcher returns a new pipeWatcher that watches the read end of writeFD for closure.
func newPipeWatcher(writeFD int) (*pipeWatcher, error) {
	epollFD, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	// Clean up after partial initialiation.
	defer func() {
		if epollFD != -1 {
			syscall.Close(epollFD)
		}
	}()

	if err := syscall.EpollCtl(epollFD, syscall.EPOLL_CTL_ADD, writeFD, &syscall.EpollEvent{}); err != nil {
		return nil, err
	}

	pw := &pipeWatcher{epollFD, make(chan struct{})}
	go func() {
		// See epoll_ctl(2): "[EPOLLERR] is also reported for the write end of a pipe
		// when the read end has been closed. epoll_wait(2) will always report for this event;
		// it is not necessary to set it in _events_."
		events := make([]syscall.EpollEvent, 1)
		if _, err := syscall.EpollWait(pw.epollFD, events, -1); err != nil {
			// Ignore errors for now; it doesn't seem like there's anything we can do.
		} else if events[0].Events == syscall.EPOLLERR {
			close(pw.readClosed)
		}
	}()

	epollFD = -1 // disarm cleanup
	return pw, nil
}

// close must be called to release resources and stop watching the FD.
func (pw *pipeWatcher) close() {
	syscall.Close(pw.epollFD)
}
