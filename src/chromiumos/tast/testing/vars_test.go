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
		t.Errorf("VarStr.Value() returns (%q, %v); want (%q, true)", val, ok, strValue)
	}
	if err := strVar.Unmarshal(strValue); err == nil {
		t.Error("failed to get error from Unmarshal")
	}
}
