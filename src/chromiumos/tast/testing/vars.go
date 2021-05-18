// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/testing"
)

// VarString define a structure for global runtime variables of string type.
type VarString struct {
	v *testing.VarString
}

// RegisterVarString creates and registers a new VarString
func RegisterVarString(name, desc, defaultValue string) *VarString {
	v := testing.NewVarString(name, desc, defaultValue)
	testing.GlobalRegistry().AddVar(v)
	return &VarString{v: v}
}

// Name returns the name of the variable.
func (v *VarString) Name() string {
	return v.v.Name()
}

// Value returns value of a variable and a flag to indicate whether the value is initialized.
func (v *VarString) Value() string {
	return v.v.Value()
}
