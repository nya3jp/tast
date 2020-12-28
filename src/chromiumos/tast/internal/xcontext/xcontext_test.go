// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package xcontext

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/clock/fakeclock"
)

// isDone checks if the Done channel of ctx is closed.
func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// waitDone waits cancellation of ctx up to 10 seconds. It returns true if the
// context is canceled; otherwise false.
func waitDone(ctx context.Context) bool {
	const timeout = 10 * time.Second

	// Use the real timer.
	tm := time.NewTimer(timeout)
	defer tm.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-tm.C:
		return false
	}
}

// useFakeClock installs a fake clock initialized with the UNIX epoch.
// restore must be called later to uninstall the fake clock.
func useFakeClock() (fclk *fakeclock.FakeClock, restore func()) {
	fclk = fakeclock.NewFakeClock(time.Unix(0, 0))
	clk = fclk
	restore = func() { clk = clock.NewClock() }
	return fclk, restore
}

func TestWithCancel(t *testing.T) {
	ctx, cancel := WithCancel(context.Background())
	defer cancel(context.Canceled)

	if isDone(ctx) {
		t.Error("On init: Done is already signaled")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("On init: Err is already set: %v", err)
	}

	// Cancel the context with wantErr.
	wantErr := errors.New("custom error")
	cancel(wantErr)

	if !isDone(ctx) {
		t.Error("After first cancel: Done is not signaled yet")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After first cancel: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the context again, which is ignored.
	cancel(errors.New("another error"))

	if !isDone(ctx) {
		t.Error("After second cancel: Done is not signaled yet")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After second cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithCancel_CanceledOnInit(t *testing.T) {
	// Create a context that is canceled with a custom error.
	wantErr := errors.New("custom error")
	ctx1, cancel1 := WithCancel(context.Background())
	cancel1(wantErr)

	// Create another context derived from the above context.
	ctx2, cancel2 := WithCancel(ctx1)
	defer cancel2(context.Canceled)

	if !isDone(ctx2) {
		t.Error("After On init: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After On init: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("another error"))

	if !isDone(ctx2) {
		t.Error("After cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithCancel_Propagate(t *testing.T) {
	ctx1, cancel1 := WithCancel(context.Background())
	defer cancel1(context.Canceled)

	ctx2, cancel2 := WithCancel(ctx1)
	defer cancel2(context.Canceled)

	// Cancel the parent context.
	wantErr := errors.New("custom error")
	cancel1(wantErr)

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After parent cancel: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("another error"))

	if !isDone(ctx2) {
		t.Error("After child cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After child cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithCancel_PropagateFromGenuineParent(t *testing.T) {
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	ctx2, cancel2 := WithCancel(ctx1)
	defer cancel2(context.Canceled)

	// Cancel the parent context.
	cancel1()

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After parent cancel: Done is not signaled")
	}
	if err := ctx2.Err(); err != context.Canceled {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, context.Canceled)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("another error"))

	if !isDone(ctx2) {
		t.Error("After child cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != context.Canceled {
		t.Errorf("After child cancel: Err mismatch: got %q, want %q", err, context.Canceled)
	}
}

func TestWithCancel_PropagateToGenuineChild(t *testing.T) {
	ctx1, cancel1 := WithCancel(context.Background())
	defer cancel1(context.Canceled)

	ctx2, cancel2 := context.WithCancel(ctx1)
	defer cancel2()

	// Cancel the parent context.
	wantErr := errors.New("custom error")
	cancel1(wantErr)

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After parent cancel: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithCancel_PropagateNoReverse(t *testing.T) {
	ctx1, cancel1 := WithCancel(context.Background())
	defer cancel1(context.Canceled)

	ctx2, cancel2 := WithCancel(ctx1)
	defer cancel2(context.Canceled)

	// Cancel the child context.
	wantErr := errors.New("custom error")
	cancel2(wantErr)

	// The parent is still alive.
	if err := ctx1.Err(); err != nil {
		t.Errorf("After child cancel: parent is canceled: %v", err)
	}

	// Cancel the parent context.
	cancel1(errors.New("another error"))

	// The child context should still return the same error.
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithCancel_Deadline(t *testing.T) {
	_, restore := useFakeClock()
	defer restore()

	dl := time.Unix(28, 0)
	ctx1, cancel1 := WithDeadline(context.Background(), dl, context.DeadlineExceeded)
	defer cancel1(context.Canceled)

	if err := ctx1.Err(); err != nil {
		t.Error("On init: Parent context is already canceled")
	}

	ctx2, cancel2 := WithCancel(ctx1)
	defer cancel2(context.Canceled)

	if d, ok := ctx2.Deadline(); !ok {
		t.Error("Deadline is not available")
	} else if !d.Equal(dl) {
		t.Errorf("Deadline mismatch: got %v, want %v", d, dl)
	}
}

func TestWithCancel_Value(t *testing.T) {
	type keyType string
	const (
		key   keyType = "foo"
		value string  = "bar"
	)

	ctx, cancel := WithCancel(context.WithValue(context.Background(), key, value))
	defer cancel(context.Canceled)

	if val := ctx.Value("baz"); val != nil {
		t.Errorf("Value(%q) = %v; want nil", "baz", val)
	}
	if val := ctx.Value(key); val != value {
		t.Errorf("Value(%q) = %v; want %v", key, val, value)
	}
}

func TestWithCancel_NilError(t *testing.T) {
	_, cancel := WithCancel(context.Background())
	defer cancel(context.Canceled)

	defer func() { recover() }()
	cancel(nil)

	t.Error("cancel(nil) did not panic")
}

func TestWithDeadline(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	dl := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx, cancel := WithDeadline(context.Background(), dl, wantErr)
	defer cancel(context.Canceled)

	if isDone(ctx) {
		t.Error("On init: Done is already signaled")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("On init: Err is already set: %v", err)
	}

	clk.WaitForNWatchersAndIncrement(28*time.Second, 1)

	if !waitDone(ctx) {
		t.Fatal("After sleep: Done is not signaled")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After sleep: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the context, which is ignored.
	cancel(errors.New("another error"))

	if err := ctx.Err(); err != wantErr {
		t.Errorf("After cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_Cancel(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	dl := time.Unix(28, 0)
	ctx, cancel := WithDeadline(context.Background(), dl, errors.New("another error"))
	defer cancel(context.Canceled)

	if isDone(ctx) {
		t.Error("On init: Done is already signaled")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("On init: Err is already set: %v", err)
	}

	// Cancel the context.
	wantErr := errors.New("custom error")
	cancel(wantErr)

	if !isDone(ctx) {
		t.Error("After cancel: Done is already signaled")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After cancel: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Sleep to trigger the deadline, but the error does not change.
	// Note: We should not use WaitForNWatchersAndIncrement here because
	// the timer of the child context is deleted on cancellation of the
	// parent context.
	clk.Increment(28 * time.Second)

	if !waitDone(ctx) {
		t.Fatal("After sleep: Done is not signaled")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After sleep: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_CanceledOnInit(t *testing.T) {
	_, restore := useFakeClock()
	defer restore()

	// Create a context that is canceled with a custom error.
	wantErr := errors.New("custom error")
	ctx1, cancel1 := WithCancel(context.Background())
	cancel1(wantErr)

	// Create another context derived from the above context.
	ctx2, cancel2 := WithDeadline(ctx1, time.Unix(28, 0), errors.New("another error"))
	defer cancel2(context.Canceled)

	if !isDone(ctx2) {
		t.Error("After On init: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After On init: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("yet another error"))

	if !isDone(ctx2) {
		t.Error("After cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_LongerDeadline(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	// Create a parent context with 28s timeout.
	dl1 := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx1, cancel1 := WithDeadline(context.Background(), dl1, wantErr)
	defer cancel1(context.Canceled)

	// Create a child context with 100s timeout.
	dl2 := time.Unix(100, 0)
	ctx2, cancel2 := WithDeadline(ctx1, dl2, errors.New("another error"))
	defer cancel2(context.Canceled)

	// The new deadline is 28s.
	if d, ok := ctx2.Deadline(); !ok {
		t.Error("Deadline is not available")
	} else if !d.Equal(dl1) {
		t.Errorf("Deadline mismatch: got %v, want %v", d, dl1)
	}

	// Advance the fake clock. Deadline exceeded error from the child
	// context is never seen.
	clk.WaitForNWatchersAndIncrement(1000*time.Second, 1)

	if !waitDone(ctx2) {
		t.Fatal("After sleep: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After sleep: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_ShorterDeadline(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	// Create a parent context with 100s timeout.
	dl1 := time.Unix(100, 0)
	ctx1, cancel1 := WithDeadline(context.Background(), dl1, errors.New("another error"))
	defer cancel1(context.Canceled)

	// Create a child context with 28s timeout.
	dl2 := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx2, cancel2 := WithDeadline(ctx1, dl2, wantErr)
	defer cancel2(context.Canceled)

	// The new deadline is 28s.
	if d, ok := ctx2.Deadline(); !ok {
		t.Error("Deadline is not available")
	} else if !d.Equal(dl2) {
		t.Errorf("Deadline mismatch: got %v, want %v", d, dl2)
	}

	// Advance the fake clock so that only the child context is canceled.
	clk.WaitForNWatchersAndIncrement(50*time.Second, 2)

	if !waitDone(ctx2) {
		t.Fatal("After sleep: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After sleep: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_Propagate(t *testing.T) {
	_, restore := useFakeClock()
	defer restore()

	ctx1, cancel1 := WithCancel(context.Background())
	defer cancel1(context.Canceled)

	dl := time.Unix(28, 0)
	ctx2, cancel2 := WithDeadline(ctx1, dl, context.DeadlineExceeded)
	defer cancel2(context.Canceled)

	// Cancel the parent context.
	wantErr := errors.New("custom error")
	cancel1(wantErr)

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After parent cancel: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("another error"))

	if !isDone(ctx2) {
		t.Error("After child cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After child cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_PropagateFromGenuineParent(t *testing.T) {
	_, restore := useFakeClock()
	defer restore()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	dl := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx2, cancel2 := WithDeadline(ctx1, dl, wantErr)
	defer cancel2(context.Canceled)

	// Cancel the parent context.
	cancel1()

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After parent cancel: Done is not signaled")
	}
	if err := ctx2.Err(); err != context.Canceled {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, context.Canceled)
	}

	// Cancel the child context, which is ignored.
	cancel2(errors.New("another error"))

	if !isDone(ctx2) {
		t.Error("After child cancel: Done is not signaled yet")
	}
	if err := ctx2.Err(); err != context.Canceled {
		t.Errorf("After child cancel: Err mismatch: got %q, want %q", err, context.Canceled)
	}
}

func TestWithDeadline_PropagateToGenuineChild(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	dl := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx1, cancel1 := WithDeadline(context.Background(), dl, wantErr)
	defer cancel1(context.Canceled)

	ctx2, cancel2 := context.WithCancel(ctx1)
	defer cancel2()

	// Advance the clock to cancel the parent context.
	clk.WaitForNWatchersAndIncrement(100*time.Second, 1)

	// The child context will be canceled with a possible delay.
	if !waitDone(ctx2) {
		t.Fatal("After clock advancement: Done is not signaled")
	}
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After clock advancement: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_PropagateNoReverse(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	ctx1, cancel1 := WithCancel(context.Background())
	defer cancel1(context.Canceled)

	dl := time.Unix(28, 0)
	wantErr := errors.New("custom error")
	ctx2, cancel2 := WithDeadline(ctx1, dl, wantErr)
	defer cancel2(context.Canceled)

	// Advance the clock to cancel the child context.
	clk.WaitForNWatchersAndIncrement(100*time.Second, 1)

	if !waitDone(ctx2) {
		t.Fatal("After clock advancement: Done is not signaled")
	}

	// The parent is still alive.
	if err := ctx1.Err(); err != nil {
		t.Errorf("After clock advancement: parent is canceled: %v", err)
	}

	// Cancel the parent context.
	cancel1(errors.New("another error"))

	// The child context should still return the same error.
	if err := ctx2.Err(); err != wantErr {
		t.Errorf("After parent cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithDeadline_Deadline(t *testing.T) {
	_, restore := useFakeClock()
	defer restore()

	dl1 := time.Unix(100, 0)
	dl2 := time.Unix(28, 0)
	dl3 := time.Unix(28, 0)

	ctx1, cancel1 := WithDeadline(context.Background(), dl1, context.DeadlineExceeded)
	defer cancel1(context.Canceled)

	ctx2, cancel2 := WithDeadline(context.Background(), dl2, context.DeadlineExceeded)
	defer cancel2(context.Canceled)

	ctx3, cancel3 := WithDeadline(context.Background(), dl3, context.DeadlineExceeded)
	defer cancel3(context.Canceled)

	for _, tc := range []struct {
		name string
		ctx  context.Context
		dl   time.Time
	}{
		{"ctx1", ctx1, dl1},
		{"ctx2", ctx2, dl2},
		{"ctx3", ctx3, dl3},
	} {
		if d, ok := tc.ctx.Deadline(); !ok {
			t.Errorf("%s: Deadline is not available", tc.name)
		} else if !d.Equal(tc.dl) {
			t.Errorf("%s: Deadline mismatch: got %v, want %v", tc.name, d, tc.dl)
		}
	}
}

func TestWithDeadline_Value(t *testing.T) {
	type keyType string
	const (
		key   keyType = "foo"
		value string  = "bar"
	)

	ctx, cancel := WithDeadline(context.WithValue(context.Background(), key, value), time.Unix(28, 0), context.DeadlineExceeded)
	defer cancel(context.Canceled)

	if val := ctx.Value("baz"); val != nil {
		t.Errorf("Value(%q) = %v; want nil", "baz", val)
	}
	if val := ctx.Value(key); val != value {
		t.Errorf("Value(%q) = %v; want %v", key, val, value)
	}
}

func TestWithDeadline_NilError(t *testing.T) {
	defer func() { recover() }()
	_, cancel := WithDeadline(context.Background(), time.Unix(0, 0), nil)
	defer cancel(context.Canceled)

	t.Error("WithDeadline(nil) did not panic")
}

func TestWithTimeout(t *testing.T) {
	clk, restore := useFakeClock()
	defer restore()

	const timeout = 28 * time.Second
	wantErr := errors.New("custom error")
	ctx, cancel := WithTimeout(context.Background(), timeout, wantErr)
	defer cancel(context.Canceled)

	if isDone(ctx) {
		t.Error("On init: Done is already signaled")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("On init: Err is already set: %v", err)
	}

	clk.WaitForNWatchersAndIncrement(timeout, 1)

	if !waitDone(ctx) {
		t.Fatal("After sleep: Done is not signaled")
	}
	if err := ctx.Err(); err != wantErr {
		t.Errorf("After sleep: Err mismatch: got %q, want %q", err, wantErr)
	}

	// Cancel the context, which is ignored.
	cancel(errors.New("another error"))

	if err := ctx.Err(); err != wantErr {
		t.Errorf("After cancel: Err mismatch: got %q, want %q", err, wantErr)
	}
}

func TestWithTimeout_NilError(t *testing.T) {
	defer func() { recover() }()
	_, cancel := WithTimeout(context.Background(), time.Second, nil)
	defer cancel(context.Canceled)

	t.Error("WithTimeout(nil) did not panic")
}
