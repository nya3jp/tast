// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"
)

// TestVarStr tests if VarStr works correctly.
func TestVarStr(t *testing.T) {
	const (
		varName     = `testVar`
		strValue    = `test value`
		defaultVal1 = `default`
	)

	strVar := NewVarString(varName, "test", defaultVal1)
	if strVar.Name() != varName {
		t.Errorf("VarStr.Name() returns %q; want %q", strVar.Name(), varName)
	}
	if strVar.Value() != defaultVal1 {
		t.Errorf("VarString.Value() returns %q as default value; want %q", strVar.Value(), defaultVal1)
	}
	if err := strVar.Unmarshal(strValue); err != nil {
		t.Error("failed to call Unmarshal: ", err)
	}
	if strVar.Value() != strValue {
		t.Errorf("VarString.Value() returns %q; want %q", strVar.Value(), strValue)
	}
}

// TestInitializeGlobalVars tests if initializeGlobalVars works correctly.
func TestInitializeGlobalVars(t *testing.T) {

	const (
		name1       = `var1`
		name2       = `var2`
		val1        = `value1`
		val2        = `value2`
		defaultVal1 = `v1`
		defaultVal2 = `v2`
	)
	values := map[string]string{
		name1: val1,
		name2: val2,
	}
	var1 := NewVarString(name1, val1, "")
	var2 := NewVarString(name2, val2, "")

	vars := map[string]Var{
		name1: var1,
		name2: var2,
	}
	stringVars := map[string]*VarString{
		name1: var1,
		name2: var2,
	}
	if err := initializeGlobalVars(vars, values); err != nil {
		t.Fatal("Failed to call initializeGlobalVars: ", err)
	}
	for k, v := range stringVars {
		expectedVal, ok := values[k]
		if !ok {
			t.Error("Failed to find variable ", k)
		}
		val := v.Value()
		if val != expectedVal {
			t.Errorf("Variable %q has value %q; want %q", k, val, expectedVal)
		}
	}
}
