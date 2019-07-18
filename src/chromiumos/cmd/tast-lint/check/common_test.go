// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
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

func TestIsTestMainFile(t *testing.T) {
	// CWD is tast/src/chromiumos/cmd/tast-lint/check
	platform := "../../../../../.."
	for _, tc := range []struct {
		testfile string
		want     bool
	}{
		{
			// Symlink is expanded when the file exists.
			testfile: filepath.Join(platform, "tast-tests/local_tests/example/pass.go"),
			want:     true,
		},
		{
			testfile: filepath.Join(platform, "tast-tests/local_tests/example/non_existent_file.go"),
			want:     false,
		},
		{
			testfile: filepath.Join(platform, "tast-tests/src/chromiumos/tast/local/bundles/cros/example/non_existent_file.go"),
			want:     true,
		},
	} {
		if got := isTestMainFile(tc.testfile); got != tc.want {
			t.Errorf("isTestMainFile(%q) = %v; want %v", tc.testfile, got, tc.want)
		}
	}
}
