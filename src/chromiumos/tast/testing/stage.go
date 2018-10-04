// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"time"
)

// stage represents part of the execution of a single test (i.e. a Test.Run call).
// Examples of stages include running a test setup function, a test function itself, or a test cleanup function.
type stage struct {
	f          stageFunc     // code to run for stage
	ctxTimeout time.Duration // used for context passed to f
	runTimeout time.Duration // used to wait for f to return; typically slightly longer than ctxTimeout
}

// stageFunc encapsulates the work done by a stage.
type stageFunc func(ctx context.Context, s *State)

// runStages runs a sequence of "stages" (i.e. functions) on behalf of Test.Run.
// If all stages finish, true is returned.
// If a stage's function has not returned before its run timeout is reached, false is returned immediately.
func runStages(ctx context.Context, s *State, stages []stage) bool {
	// stageCh is used to signal each stage's completion to the main goroutine.
	stageCh := make(chan struct{}, len(stages))

	// Run tests in a goroutine to allow the test bundle to go on to run additional tests even
	// if one test is buggy and doesn't return after its context's deadline is reached.
	go func() {
		defer close(s.ch)
		for _, st := range stages {
			rctx, rcancel := timeoutContext(ctx, st.ctxTimeout)
			defer rcancel()
			runAndRecover(st.f, rctx, s)
			stageCh <- struct{}{}
		}
	}()

	// Wait for each stage to finish.
	for _, st := range stages {
		select {
		case <-stageCh:
			// The stage finished, so wait for the next one.
		case <-time.After(st.runTimeout):
			// TODO(derat): Do more to try to kill the runaway function.
			return false
		}
	}
	return true
}

// timeoutContext returns a context and cancelation function derived from ctx with the specified timeout.
// If timeout is zero or negative (indicating an unset timeout), no timeout will be applied.
func timeoutContext(ctx context.Context, timeout time.Duration) (tctx context.Context, cancel func()) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// runAndRecover runs f synchronously with the given Context and State, and recovers and reports an error if it panics.
// f is run within a goroutine to avoid making the calling goroutine exit if the test calls s.Fatal (which calls runtime.Goexit).
func runAndRecover(f func(context.Context, *State), ctx context.Context, s *State) {
	done := make(chan struct{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.Error("Panic: ", r)
			}
			done <- struct{}{}
		}()
		f(ctx, s)
	}()
	<-done
}
