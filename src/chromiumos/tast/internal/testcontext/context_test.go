// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
	"fmt"
	"testing"
)

// TestContactVar tests if ContextVar will return runtime variables
func TestContactVar(t *testing.T) {
	const (
		unsetName        = "unset"            // registered by test but not provided at runtime
		unregName        = "unreg"            // not registered by test but provided at runtime
		strVarName       = "strVar"           // string variable that was registered and provided at runtime
		intVarName       = "intVar"           // integer variable that was registered and provided at runtime
		boolVarName      = "boolVar"          // boolean variable that was registered and provided at runtime
		wrongTypeVarName = "wrongTypeVarName" // variable with wrong type.

		strValue       = "string value"
		unregValue     = "unreg value"
		intValue       = int(8)
		boolValue      = true
		wrongTypeValue = "wrong type"
	)

	vars := map[string]string{
		unregName:        unregValue,
		strVarName:       strValue,
		intVarName:       fmt.Sprintf("%d", intValue),
		boolVarName:      fmt.Sprintf("%v", boolValue),
		wrongTypeVarName: wrongTypeValue,
	}
	ctx := NewContextWithVars(context.Background(), vars)
	globalVarConstraints := map[string]interface{}{
		unsetName:        "",
		strVarName:       "",
		intVarName:       int(0),
		boolVarName:      false,
		wrongTypeVarName: int(0),
	}
	ctx = NewContextWithGlobalVarConstraints(ctx, globalVarConstraints)

	for _, tc := range []struct {
		name  string      // name to pass to Var/RequiredVar
		value interface{} // expected variable value to be returned
		ok    bool        // expected 'ok' return value (only used if req is false)
		fatal bool        // if true, test should be aborted
	}{
		{unsetName, "", false, false},
		{unregName, "", false, true},
		{strVarName, strValue, true, false},
		{intVarName, intValue, true, false},
		{boolVarName, boolValue, true, false},
		{wrongTypeVarName, intValue, false, true},
	} {
		funcCall := fmt.Sprintf("ContextVar(%q)", tc.name)

		// Call the function in a goroutine since it may call runtime.Goexit() via Fatal.
		finished := false
		done := make(chan struct{})
		go func() {
			defer func() {
				recover()
				close(done)
			}()
			switch tc.value.(type) {
			case string:
				var value string
				ok := ContextVar(ctx, tc.name, &value)
				if ok != tc.ok || (ok && value != tc.value.(string)) {
					t.Errorf("%s = (%q, %v); want (%q, %v)", funcCall, value, ok, tc.value, tc.ok)
				}
			case bool:
				var value bool
				ok := ContextVar(ctx, tc.name, &value)
				if ok != tc.ok || (ok && value != tc.value.(bool)) {
					t.Errorf("%s = (%v, %v); want (%v, %v)", funcCall, value, ok, tc.value, tc.ok)
				}
			case int:
				var value int
				ok := ContextVar(ctx, tc.name, &value)
				if ok != tc.ok || (ok && value != tc.value.(int)) {
					t.Errorf("%s = (%v, %v); want (%v, %v)", funcCall, value, ok, tc.value, tc.ok)
				}
			}
			finished = true
		}()
		<-done

		if !finished && !tc.fatal {
			t.Error(funcCall, " aborted unexpectedly")
		} else if finished && tc.fatal {
			t.Error(funcCall, " succeeded unexpectedly")
		}

	}
}
