// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ctxutil provides convenience functions for working with context.Context objects.
package ctxutil

import (
	"context"
	"time"
)

// OptionalTimeout returns a context and cancel function derived from ctx with the specified timeout.
// If timeout is zero or negative (indicating an unset timeout), no new timeout will be applied.
func OptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// Shorten returns a context and cancel function derived from ctx with its deadline shortened by d.
// If ctx has no deadline, the returned context won't have one either. Note that if ctx's deadline is
// less than d in the future, the returned context's deadline will have already expired.
func Shorten(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	dl, ok := ctx.Deadline()
	if !ok {
		return context.WithCancel(ctx)
	}
	return context.WithDeadline(ctx, dl.Add(-d))
}
