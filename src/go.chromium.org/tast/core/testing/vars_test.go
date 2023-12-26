// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"runtime"
	gotesting "testing"

	"go.chromium.org/tast/core/internal/testing"
)

func TestRegisterVarString(t *gotesting.T) {
	varName := "testing.v1"
	value := "default"
	reg := testing.NewRegistry("bundle")
	pc, _, _, _ := runtime.Caller(0)
	v, err := registerVarString(reg, varName, value, "desc", runtime.FuncForPC(pc).Name())
	if err != nil {
		t.Fatal("Failed to call registerVarString: ", err)
	}
	if v.Name() != varName {
		t.Errorf("Function registerVarString set variable name to %q; wanted %q", v.Name(), varName)
	}
}

func TestRegisterVarStringBadName(t *gotesting.T) {
	varName := "v1"
	value := "default"
	reg := testing.NewRegistry("bundle")
	pc, _, _, _ := runtime.Caller(0)
	_, err := registerVarString(reg, varName, value, "desc", runtime.FuncForPC(pc).Name())
	if err == nil {
		t.Fatal("Failed to get an error from registerVarString when variable name is not following convention.")
	}
}

// TestVarsNoInit makes sure runtime variables are initialized before being used.
func TestVarsNoInit(t *gotesting.T) {
	varName := "testing.v1"
	value := "default"
	reg := testing.NewRegistry("bundle")
	pc, _, _, _ := runtime.Caller(0)
	v, err := registerVarString(reg, varName, value, "desc", runtime.FuncForPC(pc).Name())
	if err != nil {
		t.Fatal("Failed register variable: ", err)
	}
	if reg.VarsHaveBeenInitialized() {
		t.Fatal("Got true for reg.VarsHaveBeenInitialized; wanted false")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Uninitialized variables accessed without panic")
		}
	}()
	v.Value()
}
