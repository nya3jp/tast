// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package usercode provides utilities to interact with user-defined code.
package usercode

import (
	"context"
	"sync/atomic"
	"time"

	"chromiumos/tast/errors"
)

// PanicHandler specifies how to handle panics in SafeCall.
type PanicHandler func(val interface{})

// ErrorReporter is the interface for reporting errors. It is implemented by
// testing.State and its sibling types.
type ErrorReporter interface {
	Error(args ...interface{})
}

// ErrorOnPanic returns a PanicHandler that reports a panic via e.
func ErrorOnPanic(e ErrorReporter) PanicHandler {
	return func(val interface{}) {
		e.Error("Panic: ", val)
	}
}

// SafeCall runs a function f on a goroutine to protect callers from its
// possible bad behavior.
//
// SafeCall calls f with a context having a specified timeout. If f does not
// return before the timeout, SafeCall further waits for gracePeriod to allow
// some clean up. If f does not return after timeout + gracePeriod or ctx is
// canceled before f finishes, SafeCall abandons the goroutine and immediately
// returns an error. name is included in an error message to explain which user
// code did not return.
//
// If f panics, SafeCall calls a panic handler ph to handle it. SafeCall will
// not call ph if it decides to abandon f, even if f panics later.
//
// If f calls runtime.Goexit, it is handled just like the function returns
// normally.
//
// SafeCall returns an error only if execution of f was abandoned for some
// reasons (e.g. f ignored the timeout, ctx was canceled). In other cases, it
// returns nil.
func SafeCall(ctx context.Context, name string, timeout, gracePeriod time.Duration, ph PanicHandler, f func(ctx context.Context)) error {
	// Two goroutines race for a token below.
	// The main goroutine attempts to take a token when it sees timeout
	// or context cancellation. If it successfully takes a token, SafeCall
	// returns immediately without waiting for f to finish, and ph will
	// never be called.
	// A background goroutine attempts to take a token when it finishes
	// calling f. If it successfully takes a token, it calls recover and
	// ph (if it recovered from a panic). Until the goroutine finishes
	// SafeCall will not return.

	var token uintptr
	// takeToken returns true if it is called first time.
	takeToken := func() bool {
		return atomic.CompareAndSwapUintptr(&token, 0, 1)
	}

	done := make(chan struct{}) // closed when the background goroutine finishes
	var callErr error           // an error to be returned by a user function call

	// Start a background goroutine that calls into the user code.
	go func() {
		defer close(done)

		defer func() {
			// Always call recover to avoid crashing the process.
			val := recover()

			// Declare that the user function finished.
			// If the timeout was already reached, return immediately.
			if finishWins := takeToken(); !finishWins {
				return
			}

			// If the timeout is not reached yet, proceed with panic handling.
			// The main goroutine waits this handling to complete.
			callErr = func() error {
				// If the user code didn't panic, return success.
				if val == nil {
					return nil
				}

				// Handle forced errors.
				if fe, ok := val.(forcedError); ok {
					return fe.err
				}

				// Call ph to handle panic. Note that we must call ph on
				// this goroutine to include the panic location in the
				// stack trace.
				ph(val)

				// Always returning nil here is a bit weird, but we don't
				// have a use case of PanicHandler returning errors as of
				// today.
				return nil
			}()
		}()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		f(ctx)
	}()

	// Allow f to clean up after timeout for gracePeriod.
	tm := time.NewTimer(timeout + gracePeriod)
	defer tm.Stop()

	// Wait until the user function call finishes or the timeout is reached.
	waitErr := func() error {
		select {
		case <-done:
			return nil
		case <-tm.C:
			return errors.Errorf("%s did not return on timeout", name)
		case <-ctx.Done():
			return ctx.Err()
		}
	}()

	// Declare that the timeout was reached.
	if timeoutWins := takeToken(); timeoutWins {
		return waitErr
	}

	// If the user function call was already finished, wait for the panic
	// handling to complete and return its result.
	<-done
	return callErr
}

type forcedError struct {
	err error
}

// ForceErrorForTesting always panics. If the current function is called by
// SafeCall, it forces SafeCall to return an error.
// This function is to be used by unit tests which want to simulate SafeCall
// errors reliably.
func ForceErrorForTesting(err error) {
	panic(forcedError{err})
}
