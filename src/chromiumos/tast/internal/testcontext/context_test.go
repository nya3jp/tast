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
		validName  = "valid"  // registered by test and provided
		unsetName  = "unset"  // registered by test but not provided at runtime
		unregName  = "unreg"  // not registered by test but provided at runtime
		globalName = "global" // global defined variable

		validValue  = "valid value"
		unregValue  = "unreg value"
		globalValue = "global value"
	)

	vars := map[string]string{validName: validValue, unregName: unregValue, globalName: globalValue}
	ctx := NewContextWithVars(context.Background(), vars)
	ctx = NewContextWithAdditionalVarConstraints(ctx, []string{validName, unsetName})
	ctx = NewContextWithAdditionalVarConstraints(ctx, []string{globalName})

	for _, tc := range []struct {
		name  string // name to pass to Var/RequiredVar
		value string // expected variable value to be returned
		ok    bool   // expected 'ok' return value (only used if req is false)
		fatal bool   // if true, test should be aborted
	}{
		{validName, validValue, true, false},
		{unsetName, "", false, false},
		{unregName, "", false, true},
		{globalName, globalValue, true, false},
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
			if value, ok := ContextVar(ctx, tc.name); value != tc.value || ok != tc.ok {
				t.Errorf("%s = (%q, %v); want (%q, %v)", funcCall, value, ok, tc.value, tc.ok)
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
