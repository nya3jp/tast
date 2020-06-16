// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"time"

	"chromiumos/tast/internal/testing"
)

// stage represents part of the execution of a single test (i.e. a Test.Run call).
// Examples of stages include running a test setup function, a test function itself, or a test cleanup function.
type stage struct {
	f          stageFunc     // code to run for stage
	ctxTimeout time.Duration // used for context passed to f
	runTimeout time.Duration // used to wait for f to return; typically slightly longer than ctxTimeout
}

// stageFunc encapsulates the work done by a stage.
type stageFunc func(ctx context.Context, root *testing.RootState)

// runStages runs a sequence of "stages" (i.e. functions) on behalf of Test.Run.
// If all stages finish, true is returned.
// If a stage's function has not returned before its run timeout is reached, false is returned immediately.
func runStages(ctx context.Context, root *testing.RootState, stages []stage) bool {
	// stageCh is used to signal each stage's completion to the main goroutine.
	stageCh := make(chan struct{}, len(stages))

	// Run tests in a goroutine to allow the test bundle to go on to run additional tests even
	// if one test is buggy and doesn't return after its context's deadline is reached.
	go func() {
		defer close(stageCh)

		runStage := func(st stage) {
			rctx, rcancel := context.WithTimeout(ctx, st.ctxTimeout)
			defer rcancel()
			st.f(rctx, root)
			stageCh <- struct{}{}
		}
		for _, st := range stages {
			runStage(st)
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
	// All stages finished. Wait for the state to be closed.
	<-stageCh
	return true
}
