// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
)

// contextKeyType is the key type for objects attached to context.Context.
type contextKeyType string

// testContextKey is the key used for attaching a *TestContext to a context.Context.
const testContextKey contextKeyType = "TestContext"

// TestContextTestInfo contains information about the currently running test.
type TestContextTestInfo struct {
	// OutDir is a directory where the current test can save output files.
	OutDir string
	// SoftwareDeps is a list of software dependencies declared in the current test.
	SoftwareDeps []string
	// ServiceDeps is a list of service dependencies declared in the current test.
	ServiceDeps []string
}

// TestContext contains information accessible by using context.Context.
//
// Information in this struct is accessible from anywhere via context.Context
// and testing.Context* functions. Each member should have strong reason to be
// accessible without testing.State.
type TestContext struct {
	// Logger is a function that records a log message.
	Logger func(msg string)

	// TestInfo contains information about the current test
	TestInfo *TestContextTestInfo
}

// WithTestContext attaches TestContext to context.Context. This function can't
// be called from tests.
func WithTestContext(ctx context.Context, tc *TestContext) context.Context {
	return context.WithValue(ctx, testContextKey, tc)
}

// ContextLog formats its arguments using default formatting and logs them via
// ctx. It is intended to be used for informational logging by packages
// providing support for tests. If testing.State is available, just call
// State.Log or State.Logf instead.
func ContextLog(ctx context.Context, args ...interface{}) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok {
		return
	}
	tc.Logger(fmt.Sprint(args...))
}

// ContextLogf is similar to ContextLog but formats its arguments using fmt.Sprintf.
func ContextLogf(ctx context.Context, format string, args ...interface{}) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok {
		return
	}
	tc.Logger(fmt.Sprintf(format, args...))
}

// Logger allows test helpers to log messages when no context.Context or testing.State is available.
type Logger struct {
	sink func(msg string)
}

// Print formats its arguments using default formatting and logs them.
func (l *Logger) Print(args ...interface{}) {
	l.sink(fmt.Sprint(args...))
}

// Printf is similar to Print but formats its arguments using fmt.Sprintf.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.sink(fmt.Sprintf(format, args...))
}

// ContextLogger returns Logger from a context.
func ContextLogger(ctx context.Context) (*Logger, bool) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok {
		return nil, false
	}
	return &Logger{tc.Logger}, true
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok || tc.TestInfo == nil || tc.TestInfo.OutDir == "" {
		return "", false
	}
	return tc.TestInfo.OutDir, true
}

// ContextSoftwareDeps is similar to SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextSoftwareDeps(ctx context.Context) ([]string, bool) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok || tc.TestInfo == nil {
		return nil, false
	}

	return append([]string(nil), tc.TestInfo.SoftwareDeps...), true
}

// ContextServiceDeps is similar to ServiceDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextServiceDeps(ctx context.Context) ([]string, bool) {
	tc, ok := ctx.Value(testContextKey).(*TestContext)
	if !ok || tc.TestInfo == nil {
		return nil, false
	}

	return append([]string(nil), tc.TestInfo.ServiceDeps...), true
}
