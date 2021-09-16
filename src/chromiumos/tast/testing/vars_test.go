// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"runtime"
	gotesting "testing"

	"chromiumos/tast/internal/testing"
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
	if v.Value() != value {
		t.Errorf("Function registerVarString set variable value to %q; wanted %q", v.Value(), value)
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
