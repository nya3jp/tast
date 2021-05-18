// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testcontext provides logic to extract information from context.
package testcontext

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// varKey is the type of the key used for attaching a table of runtime variables to a context.Context.
type varKey struct{}

// allVarsKey is the type of the key used for attaching a table of all defined variables to a context.Context.
type allVarsKey struct{}

// NewContextWithVars creates a context associated with runtime variables.
func NewContextWithVars(ctx context.Context, vars map[string]string) context.Context {
	return context.WithValue(ctx, varKey{}, vars)
}

// NewContextWithGlobalVarContraints creates a context associated with runtime variable constraints.
func NewContextWithGlobalVarContraints(ctx context.Context, allVars map[string]interface{}) context.Context {
	curVarConstraints, _ := ctx.Value(allVarsKey{}).(map[string]interface{})
	if curVarConstraints == nil {
		curVarConstraints = make(map[string]interface{})
	}
	for k, v := range allVars {
		curVarConstraints[k] = v
	}
	return context.WithValue(ctx, allVarsKey{}, curVarConstraints)
}

// ContextVar returns the value for the named variable in context, which must have been registered via Vars.
func ContextVar(ctx context.Context, name string, val interface{}) bool {
	seen := false
	allVars, ok := ctx.Value(allVarsKey{}).(map[string]interface{})
	var valType interface{}
	if ok {
		valType, seen = allVars[name]
	}
	if !seen {
		panic(fmt.Sprintf("Gobal variable %q was not registered. Try adding the variables in tast-tests/src/chromiumos/tast/common/global/vars.go", name))
	}
	vars, ok := ctx.Value(varKey{}).(map[string]string)
	if !ok {
		return false
	}
	stringValue, ok := vars[name]
	if !ok {
		return false
	}
	if reflect.PtrTo(reflect.TypeOf(valType)) != reflect.TypeOf(val) {
		panic(fmt.Sprintf("Mismatched value type for %q got %v; wanted *%T\n", name, val, valType))
	}
	switch valType.(type) {
	case string:
		value := reflect.ValueOf(val)
		value.Elem().Set(reflect.ValueOf(stringValue))
		return true
	}
	if err := json.Unmarshal([]byte(stringValue), val); err != nil {
		panic(fmt.Sprintf("Failed to unmarshal global variable %q: %v", name, err))
	}
	return true
}
