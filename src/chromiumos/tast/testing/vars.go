// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import "fmt"

// Var define an interface for global runtime variable types.
type Var interface {
	Unmarshal(data string) error
	Name() string
}

// varBase defines a structure to define a global runtime variable
type varBase struct {
	name     string // name is the name of the variable.
	desc     string // desc is a description of the variable.
	hasValue bool   // hasValue indicUates whether the value has been set.
}

// Name returns the name of the variable.
func (v *varBase) Name() string {
	return v.name
}

// VarString define a structure to define a global runtime variable of string type.
type VarString struct {
	varBase
	// Values store value of the variable.
	value string
}

// NewVarString creates a new VarString
func NewVarString(name, desc string) *VarString {
	return &VarString{
		varBase: varBase{
			name: name,
			desc: desc,
		},
	}
}

// Value returns value of a variable and a flag to indicate whether the value is initialized.
func (v *VarString) Value() (string, bool) {
	return v.value, v.hasValue
}

// Unmarshal extract a string and set the value of variable type to the variable.
func (v *VarString) Unmarshal(data string) error {
	if v.hasValue {
		// Value is only allowed to be set once.
		return fmt.Errorf("it is not allowed to set value twice for %v", v.name)
	}
	v.value = data
	v.hasValue = true
	return nil
}
