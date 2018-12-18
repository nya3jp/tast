// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import "testing"

func TestExports(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

type SomeType struct{}
const SomeConst = 123
var SomeVar int = 123
func SomeFunc1() {}
func SomeFunc2() {}
`
	expects := []string{
		filename + ":3:6: Tast forbids exporting anything but one test function here; unexport type SomeType",
		filename + ":4:7: Tast forbids exporting anything but one test function here; unexport const SomeConst",
		filename + ":5:5: Tast forbids exporting anything but one test function here; unexport var SomeVar",
		filename + ":6:6: Tast forbids exporting anything but one test function here; unexport func SomeFunc1 if it is not a test function",
		filename + ":7:6: Tast forbids exporting anything but one test function here; unexport func SomeFunc2 if it is not a test function",
	}

	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestExports_ZeroFunc(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example
`
	expects := []string{
		filename + ": Tast requires exactly one test function to be exported in a test main file",
	}

	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestExports_OneFunc(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

func SomeFunc1() {} // considered the test main function
`

	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}

func TestExports_NonTestMainFile(t *testing.T) {
	const filename = "src/chromiumos/tast/local/chrome/const.go"
	const code = `package example

type SomeType struct{}
const SomeConst = 123
var SomeVar int = 123
func SomeFunc1() {}
func SomeFunc2() {}
`
	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}

func TestExports_DocFile(t *testing.T) {
	const filename = "src/chromiumos/tast/local/example/doc.go"
	const code = `// Package example demonstrates how to do things.
package example
`
	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}
