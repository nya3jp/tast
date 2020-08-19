// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
)

// contextKeyType is the key type for objects attached to context.Context.
type contextKeyType string

// entityContextKey is the key used for attaching a *EntityContext to a context.Context.
const entityContextKey contextKeyType = "EntityContext"

// EntityContext contains information about the currently running entity.
//
// Information in this struct is accessible from anywhere via context.Context
// and testing.Context* functions. Each member should have strong reason to be
// accessible without testing.*State.
type EntityContext struct {
	// OutDir is a directory where the current test can save output files.
	OutDir string
	// SoftwareDeps is a list of software dependencies declared in the current test.
	SoftwareDeps []string
	// ServiceDeps is a list of service dependencies declared in the current test.
	ServiceDeps []string
}

// WithEntityContext attaches EntityContext to context.Context. This function can't
// be called from user code.
func WithEntityContext(ctx context.Context, ec *EntityContext) context.Context {
	return context.WithValue(ctx, entityContextKey, ec)
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	ec, ok := ctx.Value(entityContextKey).(*EntityContext)
	if !ok || ec.OutDir == "" {
		return "", false
	}
	return ec.OutDir, true
}

// ContextSoftwareDeps is similar to SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextSoftwareDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(entityContextKey).(*EntityContext)
	if !ok {
		return nil, false
	}
	return append([]string(nil), ec.SoftwareDeps...), true
}

// ContextServiceDeps is similar to ServiceDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextServiceDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(entityContextKey).(*EntityContext)
	if !ok {
		return nil, false
	}
	return append([]string(nil), ec.ServiceDeps...), true
}
