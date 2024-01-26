// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// loggerKey is the type of the key used for attaching a Logger to a
// context.Context.
type loggerKey struct{}

// unexported context.Context key type to avoid collisions with other packages
type pKey int

// key used for setting the log prefix to the context
const prefixKey pKey = iota

// AttachLogger creates a new context with logger attached. Logs emitted via
// the new context are propagated to the parent context.
func AttachLogger(ctx context.Context, logger Logger) context.Context {
	if parent, ok := loggerFromContext(ctx); ok {
		logger = NewMultiLogger(logger, parent)
	}
	return context.WithValue(ctx, loggerKey{}, logger)
}

// AttachLoggerNoPropagation creates a new context with logger attached. In
// contrast to AttachLogger, logs emitted via the new context are not propagated
// to the parent context.
func AttachLoggerNoPropagation(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// HasLogger checks if any logger is attached to ctx.
func HasLogger(ctx context.Context) bool {
	_, ok := loggerFromContext(ctx)
	return ok
}

// SetLogPrefix takes in a prefix string to prepend to all logs coming through the logger
func SetLogPrefix(ctx context.Context, prefix string) context.Context {
	ctx = context.WithValue(ctx, prefixKey, prefix)
	return ctx
}

// UnsetLogPrefix removes the prefix that is prepended to all logs coming through the logger
func UnsetLogPrefix(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, prefixKey, "")
	return ctx
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
	prefix := getPrefix(ctx)
	logger.Log(level, ts, ReplaceInvalidUTF8(prefix+fmt.Sprint(args...)))
}

func logf(ctx context.Context, level Level, format string, args ...interface{}) {
	ts := time.Now() // get the time as early as possible
	logger, ok := loggerFromContext(ctx)
	if !ok {
		return
	}
	prefix := getPrefix(ctx)
	logger.Log(level, ts, ReplaceInvalidUTF8(prefix+fmt.Sprintf(format, args...)))
}

func getPrefix(ctx context.Context) string {
	prefix := ""
	if pf := ctx.Value(prefixKey); pf != nil {
		prefix = pf.(string)
	}
	return prefix
}

// ReplaceInvalidUTF8 replaces all invalid UTF-8 characters from a string.
func ReplaceInvalidUTF8(msg string) string {
	return strings.ToValidUTF8(msg, "")
}
