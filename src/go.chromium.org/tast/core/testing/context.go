// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/testcontext"
)

// ContextLog formats its arguments using default formatting and logs them via
// ctx. It is intended to be used for informational logging by packages
// providing support for tests. If testing.State is available, just call
// State.Log or State.Logf instead.
func ContextLog(ctx context.Context, args ...interface{}) {
	logging.Info(ctx, args...)
}

// ContextLogf is similar to ContextLog but formats its arguments using fmt.Sprintf.
func ContextLogf(ctx context.Context, format string, args ...interface{}) {
	logging.Infof(ctx, format, args...)
}

// ContextVLog formats its arguments using default formatting and logs them via
// ctx at the debug (verbose) level. It is intended to be used for verbose logging by packages
// providing support for tests. If testing.State is available, just call
// State.VLog or State.VLogf instead.
func ContextVLog(ctx context.Context, args ...interface{}) {
	logging.Debug(ctx, args...)
}

// ContextVLogf is similar to ContextVLog but formats its arguments using fmt.Sprintf.
func ContextVLogf(ctx context.Context, format string, args ...interface{}) {
	logging.Debugf(ctx, format, args...)
}

// Logger allows test helpers to log messages when no context.Context or testing.State is available.
type Logger struct {
	logger func(msg string)
	debug  func(msg string)
}

// Print formats its arguments using default formatting and logs them.
func (l *Logger) Print(args ...interface{}) {
	l.logger(fmt.Sprint(args...))
}

// Printf is similar to Print but formats its arguments using fmt.Sprintf.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.logger(fmt.Sprintf(format, args...))
}

// VPrint formats its arguments using default formatting and logs them at the debug (verbose) level.
func (l *Logger) VPrint(args ...interface{}) {
	l.debug(fmt.Sprint(args...))
}

// VPrintf is similar to VPrint but formats its arguments using fmt.Sprintf.
func (l *Logger) VPrintf(format string, args ...interface{}) {
	l.debug(fmt.Sprintf(format, args...))
}

// ContextLogger returns Logger from a context.
func ContextLogger(ctx context.Context) (*Logger, bool) {
	if !logging.HasLogger(ctx) {
		return nil, false
	}
	return &Logger{
		logger: func(msg string) { logging.Info(ctx, msg) },
		debug:  func(msg string) { logging.Debug(ctx, msg) },
	}, true
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	return testcontext.OutDir(ctx)
}

// ContextSoftwareDeps is similar to SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextSoftwareDeps(ctx context.Context) ([]string, bool) {
	return testcontext.SoftwareDeps(ctx)
}

// ContextEnsurePrivateAttr ensures the current entity declares a privateAttr in its metadata.
// Otherwise it will panic.
func ContextEnsurePrivateAttr(ctx context.Context, name string) {
	testcontext.EnsurePrivateAttr(ctx, name)
}

// SetLogPrefix sets log prefix for the context.
func SetLogPrefix(ctx context.Context, prefix string) context.Context {
	return logging.SetLogPrefix(ctx, prefix)
}
