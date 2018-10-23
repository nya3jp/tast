// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import "testing"

func TestTestMain(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

import (
	"context"

	"chromiumos/tast/testing"
)

const (
	Const1 int = iota
	const2
)

var (
	Var1 int
	var2 = "foo"
)

type Type1 struct{}
type type2 struct{}

func Func1() {}
func func2() {}

func Test(ctx context.Context, s *testing.State) {}
`
	expects := []string{
		filename + ":10:2: Tast forbids const Const1 to be declared at top level; move it to the test function or subpackages",
		filename + ":11:2: Tast forbids const const2 to be declared at top level; move it to the test function or subpackages",
		filename + ":15:2: Tast forbids var Var1 to be declared at top level; move it to the test function or subpackages",
		filename + ":16:2: Tast forbids var var2 to be declared at top level; move it to the test function or subpackages",
		filename + ":19:6: Tast forbids type Type1 to be declared at top level; move it to the test function or subpackages",
		filename + ":20:6: Tast forbids type type2 to be declared at top level; move it to the test function or subpackages",
		filename + ":22:6: Tast mandates exactly one exported test function to be declared in a test main file",
		filename + ":23:6: Tast forbids func func2 to be declared at top level; move it to the test function or subpackages",
		filename + ":25:6: Tast mandates exactly one exported test function to be declared in a test main file",
	}

	f, fs := parse(code, filename)
	issues := TestMain(fs, f)
	verifyIssues(t, issues, expects)
}

func TestTestMain_Pass(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

import (
	"context"

	"chromiumos/tast/testing"
)

func Test(ctx context.Context, s *testing.State) {
	const c = 123
	var v int
	type t struct{}
	f := func() {}
}
`
	f, fs := parse(code, filename)
	issues := TestMain(fs, f)
	verifyIssues(t, issues, nil)
}

func TestTestMain_NotTestMainFile(t *testing.T) {
	const filename = "src/chromiumos/tast/local/chrome/const.go"
	const code = `package chrome

const SomeNumber = 123
`
	f, fs := parse(code, filename)
	issues := TestMain(fs, f)
	verifyIssues(t, issues, nil)
}
