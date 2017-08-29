// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"net"
	"time"
)

const (
	pollInterval = 10 * time.Millisecond
)

// poll runs f repeatedly until it returns true and then returns nil.
// If ctx returns an error before then, the error is returned.
// TODO(derat): Probably ought to give f a way to return an error too.
func poll(ctx context.Context, f func() bool) error {
	for {
		if f() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(pollInterval)
	}
}

// waitForPort waits until a connection can be established to the port at addr (of the form "host:port").
func waitForPort(ctx context.Context, addr string) error {
	return poll(ctx, func() bool {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return true
		}
		return false
	})
}
