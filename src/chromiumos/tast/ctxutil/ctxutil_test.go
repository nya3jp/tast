// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ctxutil

import (
	"context"
	"testing"
	"time"
)

// runAndGetDeadline passes ctx and d to f (e.g. OptionalTimeout or Shorten) and returns
// the resulting context's deadline. A zero time is returned if no deadline is set.
func runAndGetDeadline(ctx context.Context, f func(context.Context, time.Duration) (context.Context, context.CancelFunc),
	d time.Duration) time.Time {
	ctx, cancel := f(ctx, d)
	defer cancel()

	dl, ok := ctx.Deadline()
	if ok {
		return dl
	}
	return time.Time{}
}

func TestOptionalTimeoutPositive(t *testing.T) {
	const timeout = time.Minute
	start := time.Now()
	lower := start.Add(timeout)
	upper := start.Add(timeout + time.Minute) // intentionally-high upper bound
	if dl := runAndGetDeadline(context.Background(), OptionalTimeout, timeout); dl.Before(lower) || dl.After(upper) {
		t.Errorf("OptionalTimeout returned deadline %v for %v timeout; want in range [%v, %v]", dl, timeout, lower, upper)
	}
}

func TestOptionalTimeoutZero(t *testing.T) {
	if dl := runAndGetDeadline(context.Background(), OptionalTimeout, 0); !dl.IsZero() {
		t.Errorf("OptionalTimeout returned deadline %v for 0 timeout; want 0", dl)
	}
}

func TestOptionalTimeoutNegative(t *testing.T) {
	const timeout = -time.Second
	if dl := runAndGetDeadline(context.Background(), OptionalTimeout, timeout); !dl.IsZero() {
		t.Errorf("OptionalTimeout returned deadline %v for %v timeout; want 0", dl, timeout)
	}
}

func TestShortenExistingDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	const d = 5 * time.Second // shortening duration
	orig, _ := ctx.Deadline()
	want := orig.Add(-d)
	if dl := runAndGetDeadline(ctx, Shorten, d); !dl.Equal(want) {
		t.Errorf("Shorten returned deadline %v for %v duration with original %v deadline; want %v", dl, d, orig, want)
	}
}

func TestShortenNoDeadline(t *testing.T) {
	const d = 5 * time.Second
	if dl := runAndGetDeadline(context.Background(), Shorten, d); !dl.IsZero() {
		t.Errorf("Shorten returned deadline %v for %v duration with no existing deadline; want 0", dl, d)
	}
}
