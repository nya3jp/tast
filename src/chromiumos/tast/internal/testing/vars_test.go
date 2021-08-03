// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	"testing"

	internaltest "chromiumos/tast/internal/testing"
)

// TestVarStr tests if VarStr works correctly.
func TestVarStr(t *testing.T) {
	const (
		varName     = `testVar`
		strValue    = `test value`
		defaultVal1 = `default`
	)

	strVar := internaltest.NewVarString(varName, defaultVal1, "test")
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
