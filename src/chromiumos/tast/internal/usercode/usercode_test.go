// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usercode_test

import (
	"context"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/usercode"
)

func failOnPanic(t *testing.T) usercode.PanicHandler {
	return func(val interface{}) {
		t.Error("Panic: ", val)
	}
}

func TestSafeCall(t *testing.T) {
	called := false
	if err := usercode.SafeCall(context.Background(), "foo", time.Minute, time.Minute, failOnPanic(t), func(ctx context.Context) {
		called = true
	}); err != nil {
		t.Fatal("SafeCall: ", err)
	}
	if !called {
		t.Error("Function was not called")
	}
}

func TestSafeCallTimeout(t *testing.T) {
	if err := usercode.SafeCall(context.Background(), "foo", 0, time.Minute, failOnPanic(t), func(ctx context.Context) {
		<-ctx.Done() // wait until the deadline is reached
	}); err != nil {
		t.Error("SafeCall returned an error though f returned soon after timeout")
	}
}

func TestSafeCallIgnoreTimeout(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	err := usercode.SafeCall(context.Background(), "foo", 0, 0, failOnPanic(t), func(ctx context.Context) {
		<-ch // freeze until the test finishes
	})
	if err == nil {
		t.Fatal("SafeCall returned success on timeout")
	}
	const exp = "foo did not return on timeout"
	if err.Error() != exp {
		t.Errorf("SafeCall: %v; want: %v", err, exp)
	}
}

func TestSafeCallContextCancel(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := usercode.SafeCall(ctx, "foo", time.Minute, time.Minute, failOnPanic(t), func(ctx context.Context) {
		cancel()
		<-ch // freeze until the test finishes
	})
	if err == nil {
		t.Fatal("SafeCall returned success on context cancel")
	}
	if err != context.Canceled {
		t.Errorf("SafeCall: %v; want: %v", err, context.Canceled)
	}
}

const panicMsg = "panicking"

func callPanic(ctx context.Context) {
	panic(panicMsg)
}

func TestSafeCallPanic(t *testing.T) {
	panicked := false
	onPanic := func(val interface{}) {
		panicked = true
		if s, ok := val.(string); !ok || s != panicMsg {
			t.Errorf("onPanic: got %v, want %v", val, panicMsg)
		}
		// The current call stack should contain the location where panic was called.
		stack := string(debug.Stack())
		const funcName = "callPanic"
		if !strings.Contains(stack, funcName) {
			t.Errorf("Stack does not contain %q:\n%s", funcName, stack)
		}
	}

	if err := usercode.SafeCall(context.Background(), "", time.Minute, time.Minute, onPanic, callPanic); err != nil {
		t.Fatal("SafeCall: ", err)
	}
	if !panicked {
		t.Error("PanicHandler not called")
	}
}

func TestSafeCallPanicAfterAbandon(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)

	if err := usercode.SafeCall(context.Background(), "", 0, 0, failOnPanic(t), func(ctx context.Context) {
		<-ch // freeze until the test finishes
		panic("panicking")
	}); err == nil {
		t.Fatal("SafeCall returned success on timeout")
	}
}

func TestSafeCallForceErrorForTesting(t *testing.T) {
	myError := errors.New("my error")

	if err := usercode.SafeCall(context.Background(), "", time.Hour, 0, failOnPanic(t), func(ctx context.Context) {
		usercode.ForceErrorForTesting(myError)
	}); err != myError {
		t.Errorf("SafeCall: %v; want %v", err, myError)
	}
}
