// Copyright 2020 The ChromiumOS Authors
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
	// TODO(b/229939530): Will add support for multi-DUT dependency in future.
	SoftwareDeps []string
	// ServiceDeps is a list of service dependencies declared in the current entity.
	ServiceDeps []string
	// PrivateAttr is a list of private attributes declared in the current entity.
	PrivateAttr []string
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

// EnsurePrivateAttr ensures the current entity declares a privateAttr in its metadata.
// Otherwise it will panic.
func EnsurePrivateAttr(ctx context.Context, name string) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		panic("Context is not associated with an entity")
	}
	for _, s := range ec.PrivateAttr {
		if s == name {
			return
		}
	}
	panic("Expected privateAttr " + name + " not found in the entity")
}

// PrivateAttr returns the private attributes of current entity.
func PrivateAttr(ctx context.Context) (privateAttr []string, ok bool) {
	ec, ok := ctx.Value(currentEntityKey{}).(*CurrentEntity)
	if !ok {
		return nil, ok
	}
	return ec.PrivateAttr, ok
}
