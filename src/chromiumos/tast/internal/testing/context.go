// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing privides some internal implementation of the public testing package.
package testing

import (
	"context"
	"fmt"
)

// ContextKeyType is the key type for objects attached to context.Context.
type ContextKeyType string

// TestContextKey is the key used for attaching a *TestContext to a context.Context.
const TestContextKey ContextKeyType = "TestContext"

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

// ContextLog formats its arguments using default formatting and logs them via
// ctx.
//
// This function is tested through the public version of ContextLog from
// context_test.go in the public testing package.
// internal/ssh/exec.go directly uses this function instead of the public
// version to avoid cyclic imports.
//
// TODO(oka): introduce a common way for framework and tests to emit logs via
// context.Context and move the package to internal/logging.
func ContextLog(ctx context.Context, args ...interface{}) {
	tc, ok := ctx.Value(TestContextKey).(*TestContext)
	if !ok {
		return
	}
	tc.Logger(fmt.Sprint(args...))
}
