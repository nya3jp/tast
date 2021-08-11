// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ctxutil_test

import (
	"context"
	"testing"
	"time"

	"chromiumos/tast/ctxutil"
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

func TestShortenExistingDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	const d = 5 * time.Second // shortening duration
	orig, _ := ctx.Deadline()
	want := orig.Add(-d)
	if dl := runAndGetDeadline(ctx, ctxutil.Shorten, d); !dl.Equal(want) {
		t.Errorf("Shorten returned deadline %v for %v duration with original %v deadline; want %v", dl, d, orig, want)
	}
}

func TestShortenNoDeadline(t *testing.T) {
	const d = 5 * time.Second
	if dl := runAndGetDeadline(context.Background(), ctxutil.Shorten, d); !dl.IsZero() {
		t.Errorf("Shorten returned deadline %v for %v duration with no existing deadline; want 0", dl, d)
	}
}

func TestDeadlineBefore(t *testing.T) {
	now := time.Unix(100, 0) // arbitrary
	for _, tc := range []struct {
		dl     time.Time
		before bool
	}{
		{time.Time{}, false},
		{now.Add(-time.Second), true},
		{now, false},
		{now.Add(time.Second), false},
	} {
		ctx := context.Background()
		if !tc.dl.IsZero() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, tc.dl)
			defer cancel()
		}
		if before := ctxutil.DeadlineBefore(ctx, now); before != tc.before {
			t.Errorf("DeadlineBefore(%v, %v) = %v; want %v", tc.dl, now, before, tc.before)
		}
	}
}
