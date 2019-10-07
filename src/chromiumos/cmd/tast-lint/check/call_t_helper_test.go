// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestCallTHelper(t *testing.T) {
	const code = `package main

func helperFunc1(t *testing.T){ // this function should call t.Helper()
	t.Errorf()
	helperFunc7(t)
}

func helperFunc2(t *testing.T) bool { // this function should call t.Helper()
	t.Errorf()
	helperFunc1(t)
	return true
}

func TestExample1(t *testing.T) { // not a helper function and not called by multiple non-helper functions
	t.Fatal()
	helperFunc3(t)
	TestExample3(t)
}

func helperFunc3(t *testing.T) {
	helperFunc2(t) // not calling an error/fatal functions and not called by multiple non-helper functions
	helperFunc5(t)
}

func helperFunc4(t *testing.T) {
	t.Error() // not called by multiple non-helper functions
}

func TestExample2(t *testing.T) {
	a := helperFunc2(t) // not a helper function, not calling an error/fatal functions,
	if !a { // and not called by multiple non-helper functions
		helperFunc4(t)
	}
	helperFunc5(t)
	TestExample3(t)
}

func helperFunc5(t *testing.T) {
	helperFunc6() // not calling an error/fatal functions
}

func helperFunc6() {} // don't have testing.T as its parameter

func helperFunc7(t *testing.T) {
	t.Helper() // already calls t.Helper()
	t.Fatal()
}

func TestExample3(t *testing.T) { // not a helper function
	t.Fatalf()
}
`
	const path = "example_test.go"
	f, fs := parse(code, path)
	issues := VerifyCallingTHelper(fs, f)
	expects := []string{
		path + ":3:6: testing.T.Helper should be called inside the helper function helperFunc1()",
		path + ":8:6: testing.T.Helper should be called inside the helper function helperFunc2()",
	}
	verifyIssues(t, issues, expects)
}
