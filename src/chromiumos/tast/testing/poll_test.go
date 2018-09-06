// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	gotesting "testing"
	"time"
)

func TestPoll(t *gotesting.T) {
	const expCalls = 5
	numCalls := 0
	err := Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		if numCalls < expCalls {
			return fmt.Errorf("intentional error #%d", numCalls)
		}
		return nil
	}, PollOptions{Interval: time.Millisecond})

	if err != nil {
		t.Error("Poll reported error: ", err)
	}
	if numCalls != expCalls {
		t.Errorf("Poll called function %d time(s); want %d", numCalls, expCalls)
	}
}

func TestPollCanceledContext(t *gotesting.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	numCalls := 0
	err := Poll(ctx, func(ctx context.Context) error {
		numCalls++
		return nil
	}, nil)

	if err == nil {
		t.Error("Poll didn't return expected error for canceled context")
	}
	if numCalls != 0 {
		t.Errorf("Poll called function %d time(s) for canceled context ", numCalls)
	}
}

func TestPollTimeout(t *gotesting.T) {
	// Poll should always invoke the provided function before checking whether the timeout
	// has been reached.
	numCalls := 0
	opts := &PollOptions{Timeout: time.Millisecond}
	err := Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		<-ctx.Done()
		return nil
	}, opts)
	if err != nil {
		t.Error("Poll returned error for timeout with successful func: ", err)
	}
	if numCalls != 1 {
		t.Errorf("Poll called function %d times; want 1", numCalls)
	}

	numCalls = 0
	err = Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		<-ctx.Done()
		return ctx.Err()
	}, opts)
	if err == nil {
		t.Error("Poll didn't return expected error for timeout with failing func")
	}
	if numCalls != 1 {
		t.Errorf("Poll called function %d times; want 1", numCalls)
	}
}
