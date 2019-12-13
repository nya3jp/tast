// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestPackageCommentNoComment(t *testing.T) {
	const code = `
package newpackage
`
	const path = "newpackage/newpackage.go"
	f, fs := parse(code, path)
	files := make(map[string]*ast.File)
	files[path] = f
	pkg := ast.Package{
		Name:  "newpackage",
		Files: files,
	}
	issues := PackageComment(fs, &pkg)
	expects := []string{
		path + ":2:1: document of newly created package 'newpackage' is required in one of the files in this directory",
	}
	verifyIssues(t, issues, expects)
}

func TestPackageCommentMultiOK(t *testing.T) {
	codepaths := []struct {
		code string
		path string
	}{
		{
			code: `
package newpackage
`,
			path: "newpackage/newpackage1.go",
		},
		{
			code: `
// Copyright

// Package newpackage do nothing
// do nothing
package newpackage
`,
			path: "newpackage/newpackage2.go",
		},
	}
	files := make(map[string]*ast.File)
	fs := token.NewFileSet()
	for _, pair := range codepaths {
		f, err := parser.ParseFile(fs, pair.path, pair.code, parser.ParseComments)
		if err != nil {
			t.Errorf("Cannot parse the code: %s", err)
		}
		files[pair.path] = f
	}
	pkg := ast.Package{
		Name:  "newpackage",
		Files: files,
	}
	issues := PackageComment(fs, &pkg)
	verifyIssues(t, issues, nil)
}

func TestPackageCommentMultiBad(t *testing.T) {
	codepaths := []struct {
		code string
		path string
	}{
		{
			code: `
package docisneeded
`,
			path: "docisneeded/first.go",
		},
		{
			code: `
package docisneeded

func main(){}
`,
			path: "docisneeded/second.go",
		},
		{
			code: `
package docisneeded

func main(){}
`,
			path: "docisneeded/third.go",
		},
	}
	files := make(map[string]*ast.File)
	fs := token.NewFileSet()
	for _, pair := range codepaths {
		f, err := parser.ParseFile(fs, pair.path, pair.code, parser.ParseComments)
		if err != nil {
			t.Errorf("Cannot parse the code: %s", err)
		}
		files[pair.path] = f
	}
	pkg := ast.Package{
		Name:  "docisneeded",
		Files: files,
	}
	issues := PackageComment(fs, &pkg)
	expects := []string{
		"docisneeded/third.go:2:1: document of newly created package 'docisneeded' is required in one of the files in this directory",
	}
	verifyIssues(t, issues, expects)
}
