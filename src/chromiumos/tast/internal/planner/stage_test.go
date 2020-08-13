// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	gotesting "testing"
	"time"
)

func TestRunStagesTimeout(t *gotesting.T) {
	cont := make(chan struct{}, 2)        // used to signal to first stage to exit
	defer func() { cont <- struct{}{} }() // wait until unit test is over
	ranSecond := false
	finished := runStages(context.Background(), []stage{
		{func(ctx context.Context) { <-cont }, 0, time.Millisecond},
		{func(ctx context.Context) { ranSecond = true }, 0, time.Minute},
	})
	if finished {
		t.Error("runStages reported that stages finished even though first was hanging")
	}
	if ranSecond {
		t.Error("runStages ran second stage even though first was hanging")
	}
}

func TestRunStagesContext(t *gotesting.T) {
	// Verifies that the context given to stage.f is closed before next stage.
	var stage1Ctx context.Context
	closed := false
	runStages(context.Background(), []stage{
		// Give stage 1 a long ctxTimeout so that it will stay alive in
		// stage 2 if not closed.
		stage{func(ctx context.Context) {
			// Save context for checking in next stage.
			stage1Ctx = ctx
		}, 30 * time.Second, time.Minute},
		stage{func(ctx context.Context) {
			// Check if the context in stage 1 is closed.
			select {
			case <-stage1Ctx.Done():
				closed = true
			default:
				closed = false
			}
		}, 0, time.Minute},
	})
	if !closed {
		t.Error("runStages does not close stage context before running next stage")
	}
}
