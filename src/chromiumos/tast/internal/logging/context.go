// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"context"
	"fmt"
)

// SinkFunc is the type of a function to emit log messages.
type SinkFunc = func(msg string)

// contextKey is the key type for SinkFunc attached to context.Context.
type contextKey struct{}

// NewContext creates a context associated with sink. The returned context can
// be used to call ContextLog/ContextLogf.
func NewContext(ctx context.Context, sink SinkFunc) context.Context {
	return context.WithValue(ctx, contextKey{}, sink)
}

// SinkFromContext extracts a log sink from a context.
func SinkFromContext(ctx context.Context) (SinkFunc, bool) {
	sink, ok := ctx.Value(contextKey{}).(SinkFunc)
	return sink, ok
}

// ContextLog formats its arguments using default formatting and logs them via ctx.
func ContextLog(ctx context.Context, args ...interface{}) {
	sink, ok := SinkFromContext(ctx)
	if !ok {
		return
	}
	sink(fmt.Sprint(args...))
}

// ContextLogf is similar to ContextLog but formats its arguments using fmt.Sprintf.
func ContextLogf(ctx context.Context, format string, args ...interface{}) {
	sink, ok := SinkFromContext(ctx)
	if !ok {
		return
	}
	sink(fmt.Sprintf(format, args...))
}
