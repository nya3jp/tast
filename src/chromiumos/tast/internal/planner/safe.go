// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"sync/atomic"
	"time"

	"chromiumos/tast/errors"
)

const defaultGracePeriod = 30 * time.Second // default recommended grace period for safeCall

// panicHandler specifies how to handle panics in safeCall.
type panicHandler func(val interface{})

type errorReporter interface {
	Error(args ...interface{})
}

// errorOnPanic returns a panicHandler that reports a panic via e.
func errorOnPanic(e errorReporter) panicHandler {
	return func(val interface{}) {
		e.Error("Panic: ", val)
	}
}

// safeCall runs a function f on a goroutine to protect callers from its
// possible bad behavior.
//
// safeCall calls f with a context having a specified timeout. If f does not
// return before the timeout, safeCall further waits for gracePeriod to allow
// some clean up. If f does not return after timeout + gracePeriod or ctx is
// canceled before f finishes, safeCall abandons the goroutine and immediately
// returns an error. name is included in an error message to explain which user
// code did not return.
//
// If f panics, safeCall calls a panic handler ph to handle it. safeCall will
// not call ph if it decides to abandon f, even if f panics later.
//
// If f calls runtime.Goexit, it is handled just like the function returns
// normally.
//
// safeCall returns an error only if execution of f was abandoned for some
// reasons (e.g. f ignored the timeout, ctx was canceled). In other cases, it
// returns nil.
func safeCall(ctx context.Context, name string, timeout, gracePeriod time.Duration, ph panicHandler, f func(ctx context.Context)) error {
	// Two goroutines race for a token below.
	// The main goroutine attempts to take a token when it sees timeout
	// or context cancellation. If it successfully takes a token, safeCall
	// returns immediately without waiting for f to finish, and ph will
	// never be called.
	// A background goroutine attempts to take a token when it finishes
	// calling f. If it successfully takes a token, it calls recover and
	// ph (if it recovered from a panic). Until the goroutine finishes
	// safeCall will not return.

	var token uintptr
	// takeToken returns true if it is called first time.
	takeToken := func() bool {
		return atomic.CompareAndSwapUintptr(&token, 0, 1)
	}

	done := make(chan struct{}) // closed when the background goroutine finishes

	// Start a background goroutine that calls into the user code.
	go func() {
		defer close(done)

		defer func() {
			// Always call recover to avoid crashing the process.
			val := recover()

			// If the main goroutine already returned from safeCall, do not call ph.
			if !takeToken() {
				return
			}

			// If we recovered from a panic, call ph. Note that we must call
			// ph on this goroutine to include the panic location in the
			// stack trace.
			if val != nil {
				ph(val)
			}
		}()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		f(ctx)
	}()

	// Block returning from safeCall if the background goroutine is still calling ph.
	defer func() {
		if !takeToken() {
			<-done
		}
	}()

	// Allow f to clean up after timeout for gracePeriod.
	tm := time.NewTimer(timeout + gracePeriod)
	defer tm.Stop()

	select {
	case <-done:
		return nil
	case <-tm.C:
		return errors.Errorf("%s did not return on timeout", name)
	case <-ctx.Done():
		return ctx.Err()
	}
}
