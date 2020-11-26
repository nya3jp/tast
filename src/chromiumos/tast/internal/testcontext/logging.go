// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
	"fmt"
)

// LoggerFunc is the type of a function to emit log messages.
type LoggerFunc = func(msg string)

// loggerKey is the key type for LoggerFunc attached to context.Context.
type loggerKey struct{}

// WithLogger creates a context associated with logger. The returned context can
// be used to call Log/Logf.
func WithLogger(ctx context.Context, logger LoggerFunc) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// Logger extracts a logger from a context.
func Logger(ctx context.Context) (LoggerFunc, bool) {
	logger, ok := ctx.Value(loggerKey{}).(LoggerFunc)
	return logger, ok
}

// Log formats its arguments using default formatting and logs them via ctx.
func Log(ctx context.Context, args ...interface{}) {
	logger, ok := Logger(ctx)
	if !ok {
		return
	}
	logger(fmt.Sprint(args...))
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func Logf(ctx context.Context, format string, args ...interface{}) {
	logger, ok := Logger(ctx)
	if !ok {
		return
	}
	logger(fmt.Sprintf(format, args...))
}
