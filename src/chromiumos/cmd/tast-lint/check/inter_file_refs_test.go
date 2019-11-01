// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parse2(filename1, code1, filename2, code2 string) (*token.FileSet, *ast.File, *ast.File) {
	fs := token.NewFileSet()
	f1, err := parser.ParseFile(fs, filename1, code1, 0)
	if err != nil {
		panic(err)
	}
	f2, err := parser.ParseFile(fs, filename2, code2, 0)
	if err != nil {
		panic(err)
	}
	files := map[string]*ast.File{
		filename1: f1,
		filename2: f2,
	}
	ast.NewPackage(fs, files, nil, nil)
	return fs, f1, f2
}

func TestInterFileRefs(t *testing.T) {
	const filename1 = "src/chromiumos/tast/local/bundles/cros/example/test1.go"
	const code1 = `package example

type someType struct{}
const someConst = 123
var someVar int = 123
func someFunc() {}

func Test1() {
	var x someType
	someVar = someConst
	someFunc()
}
`

	const filename2 = "src/chromiumos/tast/local/bundles/cros/example/test2.go"
	const code2 = `package example

func Test2() {
	var x someType
	someVar = someConst
	someFunc()
}
`

	expects2 := []string{
		filename2 + ":4:8: Tast forbids inter-file references in entry files; move type someType in test1.go to a shared package",
		filename2 + ":5:2: Tast forbids inter-file references in entry files; move var someVar in test1.go to a shared package",
		filename2 + ":5:12: Tast forbids inter-file references in entry files; move const someConst in test1.go to a shared package",
		filename2 + ":6:2: Tast forbids inter-file references in entry files; move func someFunc in test1.go to a shared package",
	}

	fs, f1, f2 := parse2(filename1, code1, filename2, code2)

	issues1 := InterFileRefs(fs, f1)
	verifyIssues(t, issues1, nil)

	issues2 := InterFileRefs(fs, f2)
	verifyIssues(t, issues2, expects2)
}

func TestInterFileRefs_NonTestMainFile(t *testing.T) {
	const filename1 = "src/chromiumos/tast/local/chrome/chrome1.go"
	const code1 = `package chrome

type someType struct{}
const someConst = 123
var someVar int = 123
func someFunc() {}
`

	const filename2 = "src/chromiumos/tast/local/chrome/chrome2.go"
	const code2 = `package chrome

func Foo() {
	var x someType
	someVar = someConst
	someFunc()
}
`

	fs, _, f := parse2(filename1, code1, filename2, code2)

	issues := InterFileRefs(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInterFileRefs_ForeignRefs(t *testing.T) {
	const filename = "src/chromiumos/tast/local/bundles/cros/example/test.go"
	const code = `package example

import "chromiumos/tast/local/bundles/cros/example/util"

func Test() {
	var x util.SomeType
	util.SomeVar = util.SomeConst
	util.SomeFunc()
}
`

	f, fs := parse(code, filename)

	issues := InterFileRefs(fs, f)
	verifyIssues(t, issues, nil)
}
