// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	"context"
	gotesting "testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/testing"
)

func TestServiceState(t *gotesting.T) {
	vars := []string{"var1", "var2"}

	svc := &testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {},
		Vars:     vars,
	}
	testVars := map[string]string{
		"var1": "value1",
		"var3": "value3",
	}

	ctx := context.Background()
	svcState := testing.NewServiceState(ctx, testing.NewServiceRoot(svc, testVars))

	var k, kv, v string
	var ok bool

	// 1. Variable both declared and provided by test.
	k = "var1"
	kv = "value1"
	v, ok = svcState.Var(k)
	if !ok {
		t.Errorf("Variable %q is not found in ServiceState", k)
	} else if v != kv {
		t.Errorf("Failed to get variable %q: got %q, want %q", k, v, kv)
	}

	// 2. Variable declared but not provided by test.
	k = "var2"
	v, ok = svcState.Var(k)
	if ok {
		t.Errorf("Variable %q should not return from ServiceState. Got %q", k, v)
	}

	// 3. Variable not declared, but provided by test.
	func() {
		defer func() {
			// Request an undeclared variable will cause a panic.
			if recover() == nil {
				t.Error("Undeclared variable should cause panic")
			}
		}()
		k = "var3"
		svcState.Var(k)
	}()
}
