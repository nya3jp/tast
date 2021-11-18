// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"time"
)

// heartbeatWriter writes heartbeat messages periodically to eventWriter.
type heartbeatWriter struct {
	fin chan struct{} // sending a message to this channel stops the background goroutine
}

// newHeartbeatWriter constructs a new heartbeatWriter for ew.
// Stop must be called after use to stop the background goroutine.
func newHeartbeatWriter(ew *eventWriter) *heartbeatWriter {
	const interval = time.Second

	fin := make(chan struct{})

	go func() {
		defer close(fin)

		tick := time.NewTicker(interval)
		defer tick.Stop()

		ew.Heartbeat()
		for {
			select {
			case <-tick.C:
				ew.Heartbeat()
			case <-fin:
				return
			}
		}
	}()

	return &heartbeatWriter{fin: fin}
}

// Stop stops the background goroutine to write heartbeat messages.
// Once this method returns, heartbeat messages are no longer written.
// Stop must be called exactly once; the behavior on calling it multiple times
// is undefined.
func (w *heartbeatWriter) Stop() {
	// Since the channel capacity is 0, the background goroutine will never
	// write further heartbeat messages once this send finishes.
	w.fin <- struct{}{}
}
