// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"testing"
)

func TestPackageCommentNoComment(t *testing.T) {
	const code = `
package newpackage
`
	const path = "newpackage/newpackage.go"
	f, fs := parse(code, path)
	dfmap := map[string][]*ast.File{
		"newpackage": {
			f,
		},
	}
	issues := PackageComment(fs, dfmap)
	expects := []string{
		path + ":2:1: document of newly created package 'newpackage' is required in one of the files in this directory",
	}
	verifyIssues(t, issues, expects)
}

func TestPackageCommentMulti(t *testing.T) {
	codes := []string{
		`
package newpackage
`, `
// Copyright

// Package newpackage do nothing
// do nothing
package newpackage
`,
	}
	paths := []string{
		"newpackage/newpackage1.go",
		"newpackage/newpackage2.go",
	}
	f, fs := parseMultiple(codes, paths)
	dfmap := map[string][]*ast.File{
		"newpackage": f,
	}

	codes = []string{
		`
package docisneeded
`, `
package docisneeded

func main(){}
`, `
package docisneeded

func main(){}
`,
	}
	paths = []string{
		"docisneeded/first.go",
		"docisneeded/second.go",
		"docisneeded/third.go",
	}
	f, fs = parseMultiple(codes, paths)
	dfmap["docisneeded"] = f

	issues := PackageComment(fs, dfmap)
	expects := []string{
		"docisneeded/third.go:2:1: document of newly created package 'docisneeded' is required in one of the files in this directory",
	}
	verifyIssues(t, issues, expects)
}
