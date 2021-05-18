// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// Var define an interface for global runtime variable types.
type Var interface {
	// Unmarshal convert a string to Var's value type and set it to Var.
	Unmarshal(data string) error

	// Name return the name of the variable.
	Name() string
}

func registerVar(v Var) {
	GlobalRegistry().AddVar(v)
}

// varBase defines a structure to define a global runtime variable
type varBase struct {
	name     string // name is the name of the variable.
	desc     string // desc is a description of the variable.
	hasValue bool   // hasValue indicates whether the value has been set.
}

// VarString define a structure for global runtime variables of string type.
type VarString struct {
	varBase
	// Values store value of the variable.
	value string
}

// NewVarString creates a new VarString
func NewVarString(name, desc string) *VarString {
	v := VarString{
		varBase: varBase{
			name: name,
			desc: desc,
		},
	}
	registerVar(&v)
	return &v
}

// Name returns the name of the variable.
func (v *VarString) Name() string {
	return v.name
}

// Value returns value of a variable and a flag to indicate whether the value is initialized.
func (v *VarString) Value() (string, bool) {
	return v.value, v.hasValue
}

// Unmarshal extract a string and set the value of variable type to the variable.
func (v *VarString) Unmarshal(data string) error {
	v.value = data
	v.hasValue = true
	return nil
}
