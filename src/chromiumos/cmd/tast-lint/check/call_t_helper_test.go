// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestCallTHelper(t *testing.T) {
	const code = `package main

func unitTest1(t *testing.T){
	t.Helper()
}

func unitTest2(t *testing.T) {}

func TestExample(t *testing.T) {}

func unitTest3() {}
`
	const path = "example_test.go"
	f, fs := parse(code, path)
	issues := VerifyCallingTHelper(fs, f)
	expects := []string{
		path + ":7:6: testing.T.Helper should be called inside the helper function unitTest2()",
	}
	verifyIssues(t, issues, expects)
}
