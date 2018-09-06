// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"time"
)

const defaultPollInterval = 100 * time.Millisecond

// PollOptions may be passed to Poll to configure its behavior.
type PollOptions struct {
	// Timeout specifies the maximum time to poll.
	// Non-positive values indicate no timeout (although context deadlines will still be honored).
	Timeout time.Duration
	// Interval specifies how long to sleep between polling.
	// Non-positive values indicate that a reasonable default should be used.
	Interval time.Duration
}

// Poll runs f repeatedly until f returns nil and then itself returns nil.
// If ctx returns an error before then or opts.Timeout is reached, the last error returned by f is returned.
// f should use the context passed to it, as it may have an adjusted deadline if opts.Timeout is set.
// If ctx's deadline has already been reached, f will not be invoked.
// If opts is nil, reasonable defaults are used.
//
// Polling often results in increased load and slower execution (since there's a delay between when something
// happens and when the next polling cycle notices it). It should only be used as a last resort when there's no
// other way to watch for an event. The preferred approach is to watch for events in select{} statements.
// Goroutines can be used to provide notifications over channels.
func Poll(ctx context.Context, f func(context.Context) error, opts *PollOptions) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cancel := func() {}
	if opts != nil && opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	}
	defer cancel()

	interval := defaultPollInterval
	if opts != nil && opts.Interval > 0 {
		interval = opts.Interval
	}

	for {
		var err error
		if err = f(ctx); err == nil {
			return nil
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return fmt.Errorf("%v (last error: %v)", ctx.Err(), err)
		}
	}
}
