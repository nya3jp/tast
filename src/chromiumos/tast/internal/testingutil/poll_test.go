// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testingutil_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/internal/testingutil"
)

func TestPoll(t *gotesting.T) {
	const expCalls = 5
	numCalls := 0
	err := testingutil.Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		if numCalls < expCalls {
			return fmt.Errorf("intentional error #%d", numCalls)
		}
		return nil
	}, &testingutil.PollOptions{Interval: time.Millisecond})

	if err != nil {
		t.Error("Poll reported error: ", err)
	}
	if numCalls != expCalls {
		t.Errorf("Poll called func %d time(s); want %d", numCalls, expCalls)
	}
}

func TestPollBreak(t *gotesting.T) {
	const (
		maxCalls = 5
		expCalls = 3
		errMsg   = "break the poll"
	)
	numCalls := 0
	mainError := errors.New("break the poll")
	err := testingutil.Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		if numCalls == expCalls {
			return testingutil.PollBreak(mainError)
		}
		if numCalls < maxCalls {
			return fmt.Errorf("intentional error #%d", numCalls)
		}
		return nil
	}, &testingutil.PollOptions{Interval: time.Millisecond})

	if err == nil {
		t.Error("Poll succeeded unintentionally")
	}
	if numCalls != expCalls {
		t.Errorf("Poll called func %d times(s); want %d", numCalls, expCalls)
	}
	if err != mainError {
		t.Errorf("Failed with unexpected error: got %v; want %v", err, mainError)
	}
}

func TestPollCanceledContext(t *gotesting.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	numCalls := 0
	err := testingutil.Poll(ctx, func(ctx context.Context) error {
		numCalls++
		return nil
	}, nil)

	if err == nil {
		t.Error("Poll didn't return expected error for canceled context")
	}
	if numCalls != 0 {
		t.Errorf("Poll called func %d time(s) for canceled context", numCalls)
	}
}

func TestPollTimeout(t *gotesting.T) {
	// Poll should always invoke the provided function before checking whether the timeout
	// has been reached.
	numCalls := 0
	opts := &testingutil.PollOptions{Timeout: time.Millisecond}
	err := testingutil.Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		<-ctx.Done()
		return nil
	}, opts)
	if err != nil {
		t.Error("Poll returned error for timeout with successful func: ", err)
	}
	if numCalls != 1 {
		t.Errorf("Poll called func %d times; want 1", numCalls)
	}

	numCalls = 0
	const msg = "foo"
	err = testingutil.Poll(context.Background(), func(ctx context.Context) error {
		numCalls++
		<-ctx.Done()
		return errors.New(msg)
	}, opts)
	if err == nil {
		t.Error("Poll didn't return expected error for timeout with failing func")
	} else if !strings.Contains(err.Error(), msg) {
		t.Errorf("Poll returned error %q, which doesn't contain func error %q", err.Error(), msg)
	}
	if numCalls != 1 {
		t.Errorf("Poll called func %d times; want 1", numCalls)
	}
}

func TestPollTimeoutLastError(t *gotesting.T) {
	opts := &testingutil.PollOptions{
		Timeout:  time.Minute,
		Interval: time.Nanosecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := true
	const msg = "this is a test error message"
	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		if first {
			first = false
			return errors.New(msg)
		}
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}, opts); err == nil {
		t.Error("Poll didn't return expected error for timeout with failing func")
	} else if !strings.Contains(err.Error(), msg) {
		t.Errorf("Poll returned error %q, which doesn't contain func error %q", err.Error(), msg)
	}

	ctx, cancel = context.WithCancel(context.Background())
	first = true
	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		if first {
			first = false
			return errors.New(msg)
		}
		cancel()
		<-ctx.Done()
		return testingutil.PollBreak(ctx.Err())
	}, opts); err == nil {
		t.Error("Poll didn't return expected error for timeout with failing func")
	} else if !strings.Contains(err.Error(), msg) {
		t.Errorf("Poll returned error %q, which doesn't contain func error %q", err.Error(), msg)
	}

	ctx, cancel = context.WithCancel(context.Background())
	if err := testingutil.Poll(ctx, func(ctx context.Context) error {
		cancel()
		<-ctx.Done()
		return testingutil.PollBreak(ctx.Err())
	}, opts); err == nil {
		t.Error("Poll didn't return expected error for timeout with failing func")
	} else if err != ctx.Err() {
		t.Errorf("Poll returned unexpected error: got %v; want %v", err, ctx.Err())
	}
}

func TestPollUseNonContextError(t *gotesting.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Make the function return a canned error the first time and then cancel the context
	// and return "context canceled" after that. Poll should return the canned error
	// instead of a useless one about the context.
	var msg = "foo"
	numCalls := 0
	err := testingutil.Poll(ctx, func(ctx context.Context) error {
		numCalls++
		if numCalls == 1 {
			return errors.New(msg)
		}
		cancel()
		return ctx.Err()
	}, nil)

	if err == nil {
		t.Error("Poll didn't return expected error for canceled context")
	} else if !strings.Contains(err.Error(), msg) {
		t.Errorf("Poll returned error %q, which doesn't contain func error %q", err.Error(), msg)
	}
}
