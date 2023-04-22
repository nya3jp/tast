// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ctxutil provides convenience functions for working with context.Context objects.
package ctxutil

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
)

// MaxTimeout is the maximum value of time.Duration, approximately 290 years.
//
// This value might be useful on calling some timeout-related functions.
// For example, context.WithTimeout(ctx, ctxutil.MaxTimeout) returns a new
// context with effectively the same deadline as the original context.
// (Precisely, if the original context has no deadline or a deadline later than
// MaxDuration, the new deadline is different, but it is so future that we do
// not need to distinguish them.)
const MaxTimeout = ctxutil.MaxTimeout

// Shorten returns a context and cancel function derived from ctx with its deadline shortened by d.
// If ctx has no deadline, the returned context won't have one either. Note that if ctx's deadline is
// less than d in the future, the returned context's deadline will have already expired.
func Shorten(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, d)
}

// DeadlineBefore returns true if ctx has a deadline that expires before t.
// It returns true if the deadline has already expired and false if no deadline is set.
func DeadlineBefore(ctx context.Context, t time.Time) bool {
	return ctxutil.DeadlineBefore(ctx, t)
}
