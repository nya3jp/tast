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
		filename + ":3:6: Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport type SomeType if it is not one",
		filename + ":4:7: Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport const SomeConst",
		filename + ":5:5: Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport var SomeVar",
		filename + ":6:6: Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport func SomeFunc1 if it is not one",
		filename + ":7:6: Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport func SomeFunc2 if it is not one",
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
		filename + ": Tast requires exactly one symbol (test function or service type) to be exported in an entry file",
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
	const filename = "src/chromiumos/tast/local/bundles/cros/example/doc.go"
	const code = `// Package example demonstrates how to do things.
package example
`
	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}

func TestExports_Methods(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

type someType int
func (x someType) Close() {}

func Test() {}
`
	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}

func TestExports_Service(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/service.go"
	const code = `package example

type Service struct {}
`
	f, fs := parse(code, filename)
	issues := Exports(fs, f)
	verifyIssues(t, issues, nil)
}
