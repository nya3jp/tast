// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
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
	done := make(chan interface{}, 1)
	go func() {
		defer func() {
			done <- recover()
		}()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		f(ctx)
	}()

	// Allow f to clean up after timeout for gracePeriod.
	tm := time.NewTimer(timeout + gracePeriod)
	defer tm.Stop()

	select {
	case val := <-done:
		if val != nil {
			ph(val)
		}
		return nil
	case <-tm.C:
		return errors.Errorf("%s did not return on timeout", name)
	case <-ctx.Done():
		return ctx.Err()
	}
}
