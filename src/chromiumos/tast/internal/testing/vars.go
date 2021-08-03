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

// VarString define a structure for global runtime variables of string type.
type VarString struct {
	name  string // name is the name of the variable.
	value string // Values store value of the variable.
	desc  string // desc is a description of the variable.
}

// NewVarString creates a new VarString
func NewVarString(name, defaultValue, desc string) *VarString {
	v := VarString{
		name:  name,
		value: defaultValue,
		desc:  desc,
	}
	return &v
}

// Name returns the name of the variable.
func (v *VarString) Name() string {
	return v.name
}

// Value returns value of a variable and a flag to indicate whether the value is initialized.
func (v *VarString) Value() string {
	return v.value
}

// Unmarshal extract a string and set the value of variable type to the variable.
func (v *VarString) Unmarshal(data string) error {
	v.value = data
	return nil
}
