// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"chromiumos/tast/testutil"
)

func parse(code, filename string) (*ast.File, *token.FileSet) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, filename, code, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return f, fs
}

func verifyIssues(t *testing.T, issues []*Issue, expects []string) {
	t.Helper()

	SortIssues(issues)

	if len(issues) != len(expects) {
		t.Errorf("Got %d issue(s); want %d", len(issues), len(expects))
	}

	for i := 0; i < len(issues) || i < len(expects); i++ {
		var msg, expect string
		if i < len(issues) {
			msg = issues[i].String()
		}
		if i < len(expects) {
			expect = expects[i]
		}
		if msg != expect {
			t.Errorf("Issue %d is %q; want %q", i, msg, expect)
		}
	}
}

func verifyAutoFix(t *testing.T, lintfunc func(*token.FileSet, *ast.File) []*Issue, files map[string]string, expects map[string]string) {
	t.Helper()

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)
	testutil.WriteFiles(tempDir, files)

	var issues []*Issue
	fs := token.NewFileSet()
	for filename := range files {
		f, err := parser.ParseFile(fs, filepath.Join(tempDir, filename), nil, parser.ParseComments)
		if err != nil {
			panic(err)
		}
		issues = append(issues, lintfunc(fs, f)...)
	}
	SortIssues(issues)

	for l, r := 0, len(issues)-1; l < r; l, r = l+1, r-1 {
		issues[l], issues[r] = issues[r], issues[l]
	}
	for _, i := range issues {
		if i.Fix != nil {
			i.Fix.AutoFix(i)
		}
	}

	files, err := testutil.ReadFiles(tempDir)
	if err != nil {
		panic(err)
	}
	for filename := range files {
		if files[filename] != expects[filename] {
			t.Errorf("AutoFix %s failed", filename)
		}
	}
}
