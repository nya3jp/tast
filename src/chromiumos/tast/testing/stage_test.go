// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	gotesting "testing"
	"time"
)

func TestRunStagesFatal(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{}, or.ch, &TestConfig{})
	ranSecond := false
	finished := runStages(context.Background(), s, []stage{
		{func(ctx context.Context, s *State) { s.Fatal("failed") }, 0, time.Minute},
		{func(ctx context.Context, s *State) { ranSecond = true }, 0, time.Minute},
	})
	if !finished {
		t.Error("runStages reported that stages didn't finish")
	}
	if !ranSecond {
		t.Error("runStages didn't run second stage after first failed")
	}
	if errors := getOutputErrors(or.read()); len(errors) != 1 {
		t.Errorf("runStages wrote %v errors (%+v); want 1", len(errors), errors)
	}
}

func TestRunStagesPanic(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{}, or.ch, &TestConfig{})
	ranSecond := false
	finished := runStages(context.Background(), s, []stage{
		{func(ctx context.Context, s *State) { panic("panicked") }, 0, time.Minute},
		{func(ctx context.Context, s *State) { ranSecond = true }, 0, time.Minute},
	})
	if !finished {
		t.Error("runStages reported that stages didn't finish")
	}
	if !ranSecond {
		t.Error("runStages didn't run second stage after first panicked")
	}
	if errors := getOutputErrors(or.read()); len(errors) != 1 {
		t.Errorf("runStages wrote %v errors (%+v); want 1", len(errors), errors)
	}
}

func TestRunStagesTimeout(t *gotesting.T) {
	or := newOutputReader()
	s := newState(&TestInstance{}, or.ch, &TestConfig{})

	cont := make(chan struct{}, 2)        // used to signal to first stage to exit
	defer func() { cont <- struct{}{} }() // wait until unit test is over
	ranSecond := false
	finished := runStages(context.Background(), s, []stage{
		{func(ctx context.Context, s *State) { <-cont }, 0, time.Millisecond},
		{func(ctx context.Context, s *State) { ranSecond = true }, 0, time.Minute},
	})
	if finished {
		t.Error("runStages reported that stages finished even though first was hanging")
	}
	if ranSecond {
		t.Error("runStages ran second stage even though first was hanging")
	}
}
