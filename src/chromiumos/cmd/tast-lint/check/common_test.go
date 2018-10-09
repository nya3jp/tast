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

func parse(code string) (*ast.File, *token.FileSet) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "testfile.go", code, 0)
	if err != nil {
		panic(err)
	}
	return f, fs
}

func verifyIssues(t *testing.T, issues []*Issue, expects []string) {
	if len(issues) != len(expects) {
		t.Errorf("Got %d issues; want %d", len(issues), len(expects))
		return
	}

	for i, issue := range issues {
		msg := issue.String()
		expect := expects[i]
		if msg != expect {
			t.Errorf("Issue %d is %q; want %q", i, msg, expect)
		}
	}
}
