// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package xcontext provides Context with custom errors.
package xcontext

import (
	"context"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
)

// clk is replaced in unit tests to use fake clocks.
var clk = clock.NewClock()

// CancelFunc is a function to cancel an associated context with a specified
// error. If a context is already canceled, calling this function has no effect.
// It panics if err is nil.
// Upon returning from this function, an associated context is guaranteed to be
// in a canceled state (i.e. Done channel is closed, Err returns non-nil).
type CancelFunc func(err error)

// contextImpl implements context.Context with custom errors.
type contextImpl struct {
	// parent is a parent context.
	parent context.Context

	// hasDeadline indicates whether this context has a deadline.
	hasDeadline bool

	// deadline is a deadline of this context. It is valid only when
	// hasDeadline is true.
	deadline time.Time

	// done is a channel returned by Done.
	done chan struct{}

	// req is a channel over which cancellation errors are sent. The channel
	// has capacity=1 so that sending a first error over it does not block.
	req chan error

	// errValue holds an error value returned by Err.
	errValue atomic.Value
}

// newContext returns a new context. It also starts a background goroutine to
// handle cancellation signals if needed.
//
// If deadlineErr is nil, a new context has the same deadline as its parent, and
// reqDeadline is ignored. If deadlineErr is non-nil, the deadline of a new
// context is set to reqDeadline or that of the parent context, whichever comes
// earlier.
func newContext(parent context.Context, deadlineErr error, reqDeadline time.Time) (context.Context, CancelFunc) {
	newDeadline := false
	deadline, hasDeadline := parent.Deadline()
	if deadlineErr != nil && (!hasDeadline || reqDeadline.Before(deadline)) {
		deadline = reqDeadline
		hasDeadline = true
		newDeadline = true
	}

	ctx := &contextImpl{
		parent:      parent,
		hasDeadline: hasDeadline,
		deadline:    deadline,
		done:        make(chan struct{}),
		req:         make(chan error, 1),
	}

	// Handle the cases where the new context is immediately canceled.
	if err := func() error {
		if err := parent.Err(); err != nil {
			return err
		}
		if newDeadline && !deadline.After(clk.Now()) {
			return deadlineErr
		}
		return nil
	}(); err != nil {
		ctx.errValue.Store(err)
		close(ctx.done)
		return ctx, ctx.cancel
	}

	// Start a background goroutine that handles cancellation signals.
	go func() {
		err := func() error {
			var dl <-chan time.Time
			if newDeadline {
				tm := clk.NewTimer(deadline.Sub(clk.Now()))
				defer tm.Stop()
				dl = tm.C()
			}

			select {
			case <-parent.Done():
				return parent.Err()
			case <-dl:
				return deadlineErr
			case err := <-ctx.req:
				return err
			}
		}()
		ctx.errValue.Store(err)
		close(ctx.done)
	}()

	return ctx, ctx.cancel
}

// Deadline returns the deadline of the context.
func (c *contextImpl) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, c.hasDeadline
}

// Done returns a channel that is closed on cancellation of the context.
func (c *contextImpl) Done() <-chan struct{} {
	return c.done
}

// Err returns a non-nil error if the context has been canceled.
// This method does not strictly follow the contract of the context.Context
// interface; it may return an error different from context.Canceled or
// context.DeadlineExceeded.
func (c *contextImpl) Err() error {
	if val := c.errValue.Load(); val != nil {
		return val.(error)
	}
	return nil
}

// Value returns a value associated with the context.
func (c *contextImpl) Value(key interface{}) interface{} {
	return c.parent.Value(key)
}

// cancel requests to cancel the context.
func (c *contextImpl) cancel(err error) {
	if err == nil {
		panic("xcontext: Cancel called with nil")
	}

	// Attempt to send an error to the background goroutine.
	// req has capacity=1, so at least the first send should succeed.
	select {
	case c.req <- err:
	default:
	}

	// Wait until the context is canceled.
	<-c.done
}

// WithCancel returns a context that can be canceled with arbitrary errors.
func WithCancel(parent context.Context) (context.Context, CancelFunc) {
	return newContext(parent, nil, time.Time{})
}

// WithDeadline returns a context that can be canceled with arbitrary errors on
// reaching a specified deadline. It panics if err is nil.
func WithDeadline(parent context.Context, t time.Time, err error) (context.Context, CancelFunc) {
	if err == nil {
		panic("xcontext: WithDeadline called with nil err")
	}
	return newContext(parent, err, t)
}

// WithTimeout returns a context that can be canceled with arbitrary errors on
// reaching a specified timeout. It panics if err is nil.
func WithTimeout(parent context.Context, d time.Duration, err error) (context.Context, CancelFunc) {
	if err == nil {
		panic("xcontext: WithTimeout called with nil err")
	}
	return WithDeadline(parent, clk.Now().Add(d), err)
}
