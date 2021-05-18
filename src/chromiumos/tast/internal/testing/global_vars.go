// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// GlobalVar define a structure to define a global runtime variable
type GlobalVar struct {
	// Name is the name of the variable.
	Name string

	// Desc is a description of the variable.
	Desc string
}

// globalVars is a list of declared global variables and their descriptions.
var globalVars = []GlobalVar{
	{
		Name: "example.AccessVars.globalBoolean",
		Desc: "An example variable to demonstrate how to use global varaible",
	},
}
