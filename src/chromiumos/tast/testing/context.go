// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

// ContextLog formats its arguments using default formatting and logs them via
// ctx. It is intended to be used for informational logging by packages
// providing support for tests. If testing.State is available, just call
// State.Log or State.Logf instead.
func ContextLog(ctx context.Context, args ...interface{}) {
	testing.ContextLog(ctx, args...)
}

// ContextLogf is similar to ContextLog but formats its arguments using fmt.Sprintf.
func ContextLogf(ctx context.Context, format string, args ...interface{}) {
	testing.ContextLogf(ctx, format, args...)
}

// Logger allows test helpers to log messages when no context.Context or testing.State is available.
type Logger = testing.Logger

// ContextLogger returns Logger from a context.
func ContextLogger(ctx context.Context) (*Logger, bool) {
	return testing.ContextLogger(ctx)
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	return testing.ContextOutDir(ctx)
}

// ContextSoftwareDeps is similar to SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextSoftwareDeps(ctx context.Context) ([]string, bool) {
	return testing.ContextSoftwareDeps(ctx)
}

// ContextEnsurePrivateAttr ensures the current entity declares a privateAttr in its metadata.
// Otherwise it will panic.
func ContextEnsurePrivateAttr(ctx context.Context, name string) {
	testing.ContextEnsurePrivateAttr(ctx, name)
}
