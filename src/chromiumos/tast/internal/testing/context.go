// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
)

// contextKeyType is the key type for objects attached to context.Context.
type contextKeyType string

// currentEntityKey is the key used for attaching a CurrentEntity to a context.Context.
const currentEntityKey contextKeyType = "CurrentEntity"

// CurrentEntity contains information about the currently running entity.
//
// Information in this struct is accessible from anywhere via context.Context
// and testing.Context* functions. Each member should have strong reason to be
// accessible without testing.*State.
type CurrentEntity struct {
	// OutDir is a directory where the current entity can save output files.
	OutDir string
	// HasSoftwareDeps indicates if software dependencies are available for the
	// current entity. It is true only for tests.
	HasSoftwareDeps bool
	// SoftwareDeps is a list of software dependencies declared in the current entity.
	SoftwareDeps []string
	// ServiceDeps is a list of service dependencies declared in the current entity.
	ServiceDeps []string
}

// WithCurrentEntity attaches CurrentEntity to context.Context. This function can't
// be called from user code.
func WithCurrentEntity(ctx context.Context, ec *CurrentEntity) context.Context {
	return context.WithValue(ctx, currentEntityKey, ec)
}

// ContextOutDir is similar to OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func ContextOutDir(ctx context.Context) (dir string, ok bool) {
	ec, ok := ctx.Value(currentEntityKey).(*CurrentEntity)
	if !ok || ec.OutDir == "" {
		return "", false
	}
	return ec.OutDir, true
}

// ContextSoftwareDeps is similar to SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextSoftwareDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(currentEntityKey).(*CurrentEntity)
	if !ok {
		return nil, false
	}
	if !ec.HasSoftwareDeps {
		return nil, false
	}
	return append([]string(nil), ec.SoftwareDeps...), true
}

// ContextServiceDeps is similar to ServiceDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ContextServiceDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(currentEntityKey).(*CurrentEntity)
	if !ok {
		return nil, false
	}
	return append([]string(nil), ec.ServiceDeps...), true
}
