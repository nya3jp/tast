// Copyright 2020 The Chromium OS Authors. All rights reserved.
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
// It panics if a nil err is specified.
// Upon returning from this function, an associated context is guaranteed to be
// in a canceled state (i.e. Done channel is closed, Err returns non-nil).
type CancelFunc func(err error)

// nopCancelFunc is a CancelFunc that does nothing except for nil check.
func nopCancelFunc(err error) {
	if err == nil {
		panic("econtext: Cancel called with nil")
	}
}

// baseContext implements the common logic among cancelContext and
// deadlineContext.
type baseContext struct {
	// parent is a parent context.
	parent context.Context

	// done is a channel returned by Done.
	done chan struct{}

	// req is a channel over which cancellation errors are sent. The channel
	// has 1 capacity so that sending a first error over it does not block.
	req chan error

	// errValue holds an error value returned by Err.
	errValue atomic.Value
}

// newBaseContext returns a new baseContext. It also starts a background
// goroutine that calls waitCancel to wait for cancellation signals. waitCancel
// should return when the context is requested to be canceled.
func newBaseContext(parent context.Context, waitCancel func(req <-chan error) error) *baseContext {
	newCtx := &baseContext{
		parent: parent,
		done:   make(chan struct{}),
		req:    make(chan error, 1),
	}
	go func() {
		defer close(newCtx.done)
		newCtx.errValue.Store(waitCancel(newCtx.req))
	}()
	return newCtx
}

// Done returns a channel that is closed on cancellation of the context.
func (c *baseContext) Done() <-chan struct{} {
	return c.done
}

// Err returns a non-nil error if the context has been canceled.
// This method does not strictly follow the contract of the context.Context
// interface; it may return an error different from context.Canceled or
// context.DeadlineExceeded.
func (c *baseContext) Err() error {
	if val := c.errValue.Load(); val != nil {
		return val.(error)
	}
	return nil
}

// Value returns a value associated with the context.
func (c *baseContext) Value(key interface{}) interface{} {
	return c.parent.Value(key)
}

// cancel requests to cancel the context.
func (c *baseContext) cancel(err error) {
	if err == nil {
		panic("econtext: Cancel called with nil")
	}

	select {
	case c.req <- err:
	default:
	}

	// Wait until the context is canceled.
	<-c.done
}

type cancelContext struct {
	*baseContext
}

// Deadline returns the deadline of the context.
func (c *cancelContext) Deadline() (deadline time.Time, ok bool) {
	return c.parent.Deadline()
}

// WithCancel returns a context that can be canceled with arbitrary errors.
func WithCancel(parent context.Context) (context.Context, CancelFunc) {
	// If the parent context has been canceled, return a context that has been
	// canceled with the same error.
	if parent.Err() != nil {
		return parent, nopCancelFunc
	}

	// Create a new context. It can be canceled for two reasons:
	// 1. The parent context is canceled
	// 2. CancelFunc is called
	newCtx := &cancelContext{
		baseContext: newBaseContext(parent, func(req <-chan error) error {
			select {
			case <-parent.Done():
				return parent.Err()
			case err := <-req:
				return err
			}
		}),
	}
	return newCtx, newCtx.cancel
}

type deadlineContext struct {
	*baseContext
	deadline time.Time
}

// Deadline returns the deadline of the context.
func (c *deadlineContext) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

// WithDeadline returns a context that can be canceled with arbitrary errors on
// reaching a specified deadline.
func WithDeadline(parent context.Context, t time.Time, err error) (context.Context, CancelFunc) {
	deadlineErr := err

	// If the parent context has been canceled, return a context that has
	// been canceled with the same error.
	if parent.Err() != nil {
		return WithCancel(parent)
	}
	// If the new deadline is no earlier than that of the parent context,
	// WithDeadline is equivalent to WithCancel.
	if dl, ok := parent.Deadline(); ok && !dl.After(t) {
		return WithCancel(parent)
	}

	// Create a new context. It can be canceled for three reasons:
	// 1. The parent context is canceled
	// 2. Deadline is reached
	// 3. CancelFunc is called
	newCtx := &deadlineContext{
		baseContext: newBaseContext(parent, func(req <-chan error) error {
			tm := clk.NewTimer(t.Sub(clk.Now()))
			defer tm.Stop()
			select {
			case <-parent.Done():
				return parent.Err()
			case <-tm.C():
				return deadlineErr
			case err := <-req:
				return err
			}
		}),
		deadline: t,
	}
	return newCtx, newCtx.cancel
}

// WithTimeout returns a context that can be canceled with arbitrary errors on
// reaching a specified timeout.
func WithTimeout(ctx context.Context, d time.Duration, err error) (context.Context, CancelFunc) {
	return WithDeadline(ctx, clk.Now().Add(d), err)
}
