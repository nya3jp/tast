// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"context"
	"fmt"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/testcontext"
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

// Logger allows test helpers to log messages when no context.Context or testing.State is available.
type Logger struct {
	logger func(msg string)
}

// Print formats its arguments using default formatting and logs them.
func (l *Logger) Print(args ...interface{}) {
	l.logger(fmt.Sprint(args...))
}

// Printf is similar to Print but formats its arguments using fmt.Sprintf.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.logger(fmt.Sprintf(format, args...))
}

// ContextLogger returns Logger from a context.
func ContextLogger(ctx context.Context) (*Logger, bool) {
	if !logging.HasLogger(ctx) {
		return nil, false
	}
	return &Logger{func(msg string) { logging.Info(ctx, msg) }}, true
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

// ContextEnsureLabel ensures the current entity declares a label in its metadata.
// Otherwise it will panic.
func ContextEnsureLabel(ctx context.Context, label string) {
	testcontext.EnsureLabel(ctx, label)
}
