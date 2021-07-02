// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"context"
	"fmt"
	"time"
)

// loggerKey is the type of the key used for attaching a Logger to a
// context.Context.
type loggerKey struct{}

// AttachLogger creates a new context with logger attached. If propagate is
// true, logs emitted via the new context are propagated to the parent context.
func AttachLogger(ctx context.Context, logger Logger, propagate bool) context.Context {
	if propagate {
		if parent, ok := loggerFromContext(ctx); ok {
			logger = NewMultiLogger(logger, parent)
		}
	}
	return context.WithValue(ctx, loggerKey{}, logger)
}

// HasLogger checks if any logger is attached to ctx.
func HasLogger(ctx context.Context) bool {
	_, ok := loggerFromContext(ctx)
	return ok
}

// loggerFromContext extracts a logger from a context.
// This function is unexported so that users cannot extract a logger from
// contexts. If you need to access a logger after associating it to a context,
// pass the logger explicitly to functions.
func loggerFromContext(ctx context.Context) (Logger, bool) {
	logger, ok := ctx.Value(loggerKey{}).(Logger)
	return logger, ok
}

// Info emits a log with info level.
func Info(ctx context.Context, args ...interface{}) {
	log(ctx, LevelInfo, args...)
}

// Infof is similar to Info but formats its arguments using fmt.Sprintf.
func Infof(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, LevelInfo, format, args...)
}

// Debug emits a log with debug level.
func Debug(ctx context.Context, args ...interface{}) {
	log(ctx, LevelDebug, args...)
}

// Debugf is similar to Debug but formats its arguments using fmt.Sprintf.
func Debugf(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, LevelDebug, format, args...)
}

func log(ctx context.Context, level Level, args ...interface{}) {
	ts := time.Now() // get the time as early as possible
	logger, ok := loggerFromContext(ctx)
	if !ok {
		return
	}
	logger.Log(level, ts, fmt.Sprint(args...))
}

func logf(ctx context.Context, level Level, format string, args ...interface{}) {
	ts := time.Now() // get the time as early as possible
	logger, ok := loggerFromContext(ctx)
	if !ok {
		return
	}
	logger.Log(level, ts, fmt.Sprintf(format, args...))
}
