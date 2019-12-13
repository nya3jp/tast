// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package control

import (
	"sync"
	"time"
)

// HeartbeatWriter writes heartbeat messages periodically to MessageWriter.
type HeartbeatWriter struct {
	mu     sync.Mutex
	closed bool
	fin    chan struct{} // sending a message to this channel stops the background goroutine
}

// NewHeartbeatWriter constructs a new HeartbeatWriter for mw.
// d is the interval at which heartbeat messages are written. if d is non-positive,
// no heartbeat message will be written. In any case, Stop must be called after use
// to stop the background goroutine.
func NewHeartbeatWriter(mw *MessageWriter, d time.Duration) *HeartbeatWriter {
	fin := make(chan struct{})

	go func() {
		defer close(fin)

		if d <= 0 {
			<-fin
			return
		}

		tick := time.NewTicker(d)
		defer tick.Stop()

		mw.WriteMessage(&Heartbeat{Time: time.Now()})
		for {
			select {
			case <-tick.C:
				mw.WriteMessage(&Heartbeat{Time: time.Now()})
			case <-fin:
				return
			}
		}
	}()

	return &HeartbeatWriter{fin: fin}
}

// Stop stops the background goroutine to write heartbeat messages.
// Once this method returns, heartbeat messages are no longer written.
// Be aware that this method may block if the writer is blocking.
// It is safe to call Stop multiple times.
func (w *HeartbeatWriter) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// Since the channel capacity is 0, the background goroutine will never
	// write further heartbeat messages once this send finishes.
	w.fin <- struct{}{}
	w.closed = true
}
