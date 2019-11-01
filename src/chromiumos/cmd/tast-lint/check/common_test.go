// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func parse(code, filename string) (*ast.File, *token.FileSet) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, filename, code, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return f, fs
}

func verifyIssues(t *testing.T, issues []*Issue, want []string) {
	t.Helper()

	SortIssues(issues)

	got := make([]string, len(issues))
	for i, issue := range issues {
		got[i] = issue.String()
	}

	if diff := cmp.Diff(got, want, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Issues mismatch (-got +want):\n%s", diff)
	}
}
