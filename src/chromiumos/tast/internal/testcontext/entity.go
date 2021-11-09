// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcontext provides logic to extract information from context.
package testcontext

import (
	"context"
)

// currentEntityKey is the type of the key used for attaching a CurrentEntity
// to a context.Context.
type currentEntityKey struct{}

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
	// Labels is a list of labels declared in the current entity.
	Labels []string
}

// WithCurrentEntity attaches CurrentEntity to context.Context. This function can't
// be called from user code.
func WithCurrentEntity(ctx context.Context, ec *CurrentEntity) context.Context {
	return context.WithValue(ctx, currentEntityKey{}, ec)
}

// OutDir is similar to testing.State.OutDir but takes context instead. It is intended to be
// used by packages providing support for tests that need to write files.
func OutDir(ctx context.Context) (dir string, ok bool) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok || ec.OutDir == "" {
		return "", false
	}
	return ec.OutDir, true
}

// SoftwareDeps is similar to testing.State.SoftwareDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func SoftwareDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		return nil, false
	}
	if !ec.HasSoftwareDeps {
		return nil, false
	}
	return append([]string(nil), ec.SoftwareDeps...), true
}

// ServiceDeps is similar to testing.State.ServiceDeps but takes context instead.
// It is intended to be used by packages providing support for tests that want to
// make sure tests declare proper dependencies.
func ServiceDeps(ctx context.Context) ([]string, bool) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		return nil, false
	}
	return append([]string(nil), ec.ServiceDeps...), true
}

// EnsureLabel ensures the current entity declares a label in its metadata.
// Otherwise it will panic.
func EnsureLabel(ctx context.Context, label string) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		panic("Context is not associated with an entity")
	}
	for _, s := range ec.Labels {
		if s == label {
			return
		}
	}
	panic("Expected label " + label + " not found in the entity")
}

// Labels returns the labels of current entity.
func Labels(ctx context.Context) (labels []string, ok bool) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		return nil, ok
	}
	return ec.Labels, ok
}
