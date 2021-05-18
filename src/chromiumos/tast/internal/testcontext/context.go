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

// NewContextWithAdditionalVarContraints creates a context associated with runtime variable contraints.
func NewContextWithAdditionalVarContraints(ctx context.Context, allVars []string) context.Context {
	curVarContraints, _ := ctx.Value(allVarsKey{}).([]string)
	return context.WithValue(ctx, allVarsKey{}, append(curVarContraints, allVars...))
}

// ContextVar returns the value for the named variable in context, which must have been registered via Vars.
func ContextVar(ctx context.Context, name string) (string, bool) {
	seen := false
	allVars, ok := ctx.Value(allVarsKey{}).([]string)
	for _, n := range allVars {
		if n == name {
			seen = true
			break
		}
	}
	if !seen {
		panic(fmt.Sprintf("Variable %q was not registered in testing.Test.Vars or in fixtures. Try adding the line 'Vars: []string{%q},' to your testing.Test{}", name, name))
	}
	vars, ok := ctx.Value(varKey{}).(map[string]string)
	if !ok {
		return "Empty table", false
	}
	val, ok := vars[name]
	return val, ok
}
