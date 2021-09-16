// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"strings"

	"chromiumos/tast/caller"
	"chromiumos/tast/internal/packages"
	"chromiumos/tast/internal/testing"
)

// VarString define a structure for global runtime variables of string type.
type VarString struct {
	v *testing.VarString
}

// RegisterVarString creates and registers a new VarString
func RegisterVarString(name, defaultValue, desc string) *VarString {
	reg := testing.GlobalRegistry()
	callerFunc := caller.Get(2)
	v, err := registerVarString(reg, name, defaultValue, desc, callerFunc)
	if err != nil {
		reg.RecordError(err)
	}
	return v
}

// registerVarString creates and registers a new VarString
func registerVarString(reg *testing.Registry, name, defaultValue, desc, callerFunc string) (*VarString, error) {
	if !checkVarName(callerFunc, name) {
		return nil, fmt.Errorf("Global runtime variable %q does not follow naming convention <pkg>.<rest_of_name>", name)
	}
	v := testing.NewVarString(name, defaultValue, desc)
	reg.AddVar(v)
	return &VarString{v: v}, nil
}

// Name returns the name of the variable.
func (v *VarString) Name() string {
	return v.v.Name()
}

// Value returns value of the variable.
func (v *VarString) Value() string {
	return v.v.Value()
}

// checkVarName check if variable name follows naming convention.
func checkVarName(funcName, name string) bool {
	pkg, _ := packages.SplitFuncName(funcName)
	base := pkg[strings.LastIndex(pkg, "/")+1:]
	return strings.HasPrefix(name, base+".")
}
