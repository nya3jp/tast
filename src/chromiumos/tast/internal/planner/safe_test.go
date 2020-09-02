// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"context"
	"testing"
	"time"
)

func failOnPanic(t *testing.T) panicHandler {
	return func(val interface{}) {
		t.Error("Panic: ", val)
	}
}

func TestSafeCall(t *testing.T) {
	called := false
	if err := safeCall(context.Background(), "foo", time.Minute, time.Minute, failOnPanic(t), func(ctx context.Context) {
		called = true
	}); err != nil {
		t.Fatal("safeCall: ", err)
	}
	if !called {
		t.Error("Function was not called")
	}
}

func TestSafeCallTimeout(t *testing.T) {
	if err := safeCall(context.Background(), "foo", 0, time.Minute, failOnPanic(t), func(ctx context.Context) {
		<-ctx.Done() // wait until the deadline is reached
	}); err != nil {
		t.Error("safeCall returned an error though f returned soon after timeout")
	}
}

func TestSafeCallIgnoreTimeout(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	err := safeCall(context.Background(), "foo", 0, 0, failOnPanic(t), func(ctx context.Context) {
		<-ch // freeze until the test finishes
	})
	if err == nil {
		t.Fatal("safeCall returned success on timeout")
	}
	const exp = "foo did not return on timeout"
	if err.Error() != exp {
		t.Errorf("safeCall: %v; want: %v", err, exp)
	}
}

func TestSafeCallContextCancel(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := safeCall(ctx, "foo", time.Minute, time.Minute, failOnPanic(t), func(ctx context.Context) {
		cancel()
		<-ch // freeze until the test finishes
	})
	if err == nil {
		t.Fatal("safeCall returned success on context cancel")
	}
	if err != context.Canceled {
		t.Errorf("safeCall: %v; want: %v", err, context.Canceled)
	}
}

func TestSafeCallPanic(t *testing.T) {
	const msg = "panicking"

	panicked := false
	onPanic := func(val interface{}) {
		panicked = true
		if s, ok := val.(string); !ok || s != msg {
			t.Errorf("onPanic: got %v, want %v", val, msg)
		}
	}

	if err := safeCall(context.Background(), "", time.Minute, time.Minute, onPanic, func(ctx context.Context) {
		panic(msg)
	}); err != nil {
		t.Fatal("safeCall: ", err)
	}
	if !panicked {
		t.Error("panicHandler not called")
	}
}

func TestSafeCallPanicAfterAbandon(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	if err := safeCall(context.Background(), "", 0, 0, failOnPanic(t), func(ctx context.Context) {
		<-ch // freeze until the test finishes
		panic("panicking")
	}); err == nil {
		t.Fatal("safeCall returned success on timeout")
	}
}
