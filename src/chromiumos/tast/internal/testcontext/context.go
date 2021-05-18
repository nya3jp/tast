// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcontext provides logic to extract information from context.
package testcontext

import (
	"context"
	"fmt"
)

// varKey is the type of the key used for attaching a table of runtime variables to a context.Context.
type varKey struct{}

// allVarsKey is the type of the key used for attaching a table of all defined variables to a context.Context.
type allVarsKey struct{}

// NewContextWithVars creates a context associated with runtime variables.
func NewContextWithVars(ctx context.Context, vars map[string]string) context.Context {
	return context.WithValue(ctx, varKey{}, vars)
}

// NewContextWithAdditionalVarConstraints creates a context associated with runtime variable constraints.
func NewContextWithAdditionalVarConstraints(ctx context.Context, allVars []string) context.Context {
	curVarConstraints, _ := ctx.Value(allVarsKey{}).(map[string]struct{})
	if curVarConstraints == nil {
		curVarConstraints = make(map[string]struct{})
	}
	for _, v := range allVars {
		curVarConstraints[v] = struct{}{}
	}
	return context.WithValue(ctx, allVarsKey{}, curVarConstraints)
}

// ContextVar returns the value for the named variable in context, which must have been registered via Vars.
func ContextVar(ctx context.Context, name string) (string, bool) {
	seen := false
	allVars, ok := ctx.Value(allVarsKey{}).(map[string]struct{})
	if ok {
		_, seen = allVars[name]
	}
	if !seen {
		panic(fmt.Sprintf("Gobal Variable %q was not registered. Try adding the variables in tast-tests/src/chromiumos/tast/common/global/vars.go", name))
	}
	vars, ok := ctx.Value(varKey{}).(map[string]string)
	if !ok {
		return "Empty table", false
	}
	val, ok := vars[name]
	return val, ok
}
