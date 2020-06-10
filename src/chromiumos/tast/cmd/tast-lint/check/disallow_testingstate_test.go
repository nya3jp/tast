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
func C(s testing.State) {}
func D(s ***testing.State) {}
`
	const path = "/src/chromiumos/tast/local/test1.go"
	f, fs := parse(code, path)
	issues := VerifyTestingStateParam(fs, f)
	expects := []string{
		path + ":2:30: 'testing.State' should not be used in support packages, except for precondition implementation",
		path + ":4:25: 'testing.State' should not be used in support packages, except for precondition implementation",
		path + ":8:10: 'testing.State' should not be used in support packages, except for precondition implementation",
		path + ":9:10: 'testing.State' should not be used in support packages, except for precondition implementation",
	}
	verifyIssues(t, issues, expects)
}

// TestTestingStateAllowed checks that particular uses of testing.State are
// valid and not warned.
func TestTestingStateAllowed(t *testing.T) {
	const code = `package main
func A(s *testing.State) {}
`
	const path = "/src/chromiumos/tast/local/bundlemain/main.go"
	f, fs := parse(code, path)
	issues := VerifyTestingStateParam(fs, f)
	verifyIssues(t, issues, nil)
}

// TestTestingStateStruct checks VerifyTestingStateStruct surely returns issues
// if there are testing.State inside struct types.
func TestTestingStateStruct(t *testing.T) {
	const code = `package main
type NewStruct struct{
	foo testing.State
	bar context.Context
	baz *testing.State
	qux courge.grault
	quux string
	quuz **testing.State
	corge ****testing.State
}
func main() {
	var grault NewStruct
	return grault
}
`
	const path = "hoge.go"
	f, fs := parse(code, path)
	issues := VerifyTestingStateStruct(fs, f)
	expects := []string{
		path + ":3:6: 'testing.State' should not be stored inside a struct type",
		path + ":5:6: 'testing.State' should not be stored inside a struct type",
		path + ":8:7: 'testing.State' should not be stored inside a struct type",
		path + ":9:8: 'testing.State' should not be stored inside a struct type",
	}
	verifyIssues(t, issues, expects)
}
