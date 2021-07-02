// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"

	"chromiumos/tast/internal/logging"
)

// LoggerFunc is the type of a function to emit log messages.
type LoggerFunc = func(msg string)

// WithLogger creates a context associated with logger. The returned context can
// be used to call Log/Logf.
func WithLogger(ctx context.Context, logger LoggerFunc) context.Context {
	return logging.AttachLoggerNoPropagation(ctx, logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(logger)))
}

// Logger extracts a logger from a context.
func Logger(ctx context.Context) (LoggerFunc, bool) {
	if !logging.HasLogger(ctx) {
		return nil, false
	}
	return func(msg string) {
		logging.Info(ctx, msg)
	}, true
}

// Log formats its arguments using default formatting and logs them via ctx.
func Log(ctx context.Context, args ...interface{}) {
	logging.Info(ctx, args...)
}

// Logf is similar to Log but formats its arguments using fmt.Sprintf.
func Logf(ctx context.Context, format string, args ...interface{}) {
	logging.Infof(ctx, format, args...)
}
