// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestTestingState(t *testing.T) {
	const code = `package main
func A(id int, a *float64, s *testing.State) {}
func B() {
	fn(decl, func(a int, s *testing.State) bool {
		return false
	})
}
`
	const path = "/src/chromiumos/tast/local/test1.go"
	f, fs := parse(code, path)
	issues := VerifyTestingState(fs, f)
	expects := []string{
		path + ":2:31: 'testing.State' should not be used in support packages, except for precondition implementation",
		path + ":4:26: 'testing.State' should not be used in support packages, except for precondition implementation",
	}
	verifyIssues(t, issues, expects)
}

// TestTestingStatePrecondition checks that testing.State in precondition implementation
// is valid use so that it shouldn't be warned.
func TestTestingStatePrecondition(t *testing.T) {
	const code = `package main
func A(s *testing.State) {}
`
	const path = "/src/chromiumos/tast/local/arc/pre.go"
	f, fs := parse(code, path)
	issues := VerifyTestingState(fs, f)
	expects := []string{}
	verifyIssues(t, issues, expects)
}
