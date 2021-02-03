// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"context"
)

// Key type for objects attached to context.Context objects.
type contextKeyType string

// Key used for attaching a Logger to a context.Context.
var loggerKey contextKeyType = "logger"

// NewContext returns a new context derived from ctx that carries value lg.
func NewContext(ctx context.Context, lg *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, lg)
}

// FromContext returns the Logger value stored in ctx, if any.
func FromContext(ctx context.Context) (*Logger, bool) {
	lg, ok := ctx.Value(loggerKey).(*Logger)
	return lg, ok
}
