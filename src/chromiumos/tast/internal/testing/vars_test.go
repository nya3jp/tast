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
		varName  = `testVar`
		strValue = `test value`
	)

	strVar := NewVarString(varName, "test")
	if strVar.Name() != varName {
		t.Errorf("VarStr.Name() returns %q; want %q", strVar.Name(), varName)
	}
	val, ok := strVar.Value()
	if ok != false {
		t.Errorf("VarStr.Value() returns %v; want false", ok)
	}
	if err := strVar.Unmarshal(strValue); err != nil {
		t.Error("failed to call Unmarshal: ", err)
	}
	val, ok = strVar.Value()
	if val != strValue || ok != true {
		t.Errorf("VarString.Value() returns (%q, %v); want (%q, true)", val, ok, strValue)
	}
}

// TestInitializeGlobalVars tests if initializeGlobalVars works correctly.
func TestInitializeGlobalVars(t *testing.T) {

	const (
		name1 = `var1`
		name2 = `var2`
		val1  = `value1`
		val2  = `value2`
	)
	values := map[string]string{
		name1: val1,
		name2: val2,
	}
	var1 := NewVarString(name1, val1)
	var2 := NewVarString(name2, val2)

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
		val, initialized := v.Value()
		if !initialized {
			t.Errorf("Variable %q was not initialized", k)
		}
		if val != expectedVal {
			t.Errorf("Variable %q has value %q; want %q", k, val, expectedVal)
		}
	}
}
