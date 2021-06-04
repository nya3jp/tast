// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcontext provides logic to extract information from context.
package testcontext

import (
	"context"
)

// varKey is the type of the key used for attaching a table of runtime variables to a context.Context.
type varKey struct{}

// NewContextWithVars creates a context associated with runtime variables.
func NewContextWithVars(ctx context.Context, vars map[string]string) context.Context {
	return context.WithValue(ctx, varKey{}, vars)
}

// ContextVar returns the value for the named variable in context, which must have been registered via Vars.
func ContextVar(ctx context.Context, name string) (string, bool) {
	vars, ok := ctx.Value(varKey{}).(map[string]string)
	if !ok {
		return "Empty table", false
	}
	val, ok := vars[name]
	return val, ok
}
