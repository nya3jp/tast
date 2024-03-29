// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testingutil

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

const defaultPollInterval = 100 * time.Millisecond

// PollOptions provides testing.PollOptions.
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

// PollBreak implements testing.PollBreak.
func PollBreak(err error) error {
	return &pollBreak{err}
}

// Poll implements testing.Poll.
func Poll(ctx context.Context, f func(context.Context) error, opts *PollOptions) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "poll fails before actually running the function")
	}

	timeout := ctxutil.MaxTimeout
	timeoutLog := "with no set timeout"
	if opts != nil && opts.Timeout > 0 {
		timeout = opts.Timeout
		timeoutLog = fmt.Sprintf("with timeout %v", timeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
			if ctx.Err() != nil && lastErr != nil {
				return errors.Wrapf(lastErr, "%s during a poll %v; last error follows", e.err, timeoutLog)
			}
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
			if lastErr != nil {
				return errors.Wrapf(lastErr, "%s during a poll %v; last error follows", ctx.Err(), timeoutLog)
			}
			return errors.Wrap(ctx.Err(), "poll fails before the first execution of the given function completes")
		}
	}
}
