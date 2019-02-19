// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
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

// pollBreak is a wrapper of error to terminate the Poll immediately.
type pollBreak struct {
	err error
}

// Error implementation of pollBreak. However, it is not expected that this
// is used directly, since pollBreak is not returned to callers.
func (b *pollBreak) Error() string {
	return b.err.Error()
}

// PollBreak creates an error wrapping err that may be returned from a
// function passed to Poll to terminate polling immediately. For example:
//
// err := testing.Poll(ctx, func(ctx context.Context) error {
//   if err := mustSucceed(ctx); err != nil {
//     return testing.PollBreak(err)
//   }
//   ...
// }
func PollBreak(err error) error {
	return &pollBreak{err}
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
// If an error wrapped by PollBreak is returned, then it
// immediately terminates the polling, and returns the unwrapped error.
func Poll(ctx context.Context, f func(context.Context) error, opts *PollOptions) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var timeout time.Duration
	if opts != nil {
		timeout = opts.Timeout
	}
	ctx, cancel := ctxutil.OptionalTimeout(ctx, timeout)
	defer cancel()

	interval := defaultPollInterval
	if opts != nil && opts.Interval > 0 {
		interval = opts.Interval
	}

	var lastErr error
	for {
		var err error
		if err = f(ctx); err == nil {
			return nil
		}

		if e, ok := err.(*pollBreak); ok {
			return e.err
		}

		// If f honors ctx's deadline, it may return a "context deadline exceeded" error
		// if the deadline is reached while is running. To avoid returning a useless
		// "context deadline exceeded; last error follows: context deadline exceeded)" error below,
		// save the last error that is returned before the deadline is reached.
		if lastErr == nil || ctx.Err() == nil {
			lastErr = err
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return errors.Wrapf(lastErr, "%s; last error follows", ctx.Err())
		}
	}
}
