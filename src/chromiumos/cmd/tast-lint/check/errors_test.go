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

func verifyIssues(t *testing.T, fs *token.FileSet, issues []*Issue, expects []string) {
	if len(issues) != len(expects) {
		t.Errorf("Got %d issues; want %d", len(issues), len(expects))
		return
	}

	for i, issue := range issues {
		msg := issue.String(fs)
		expect := expects[i]
		if msg != expect {
			t.Errorf("Issue %d is %q; want %q", i, msg, expect)
		}
	}
}

func TestErrorsImports(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"errors"

	"chromiumos/tast/errors"

	"github.com/pkg/errors"
)
`
	expects := []string{
		"testfile.go:5:2: chromiumos/tast/errors package should be used instead of errors package",
		"testfile.go:9:2: chromiumos/tast/errors package should be used instead of github.com/pkg/errors package",
	}

	f, fs := parse(code)
	issues := ErrorsImports(f)
	verifyIssues(t, fs, issues, expects)
}

func TestFmtPrintf(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
}
`
	expects := []string{
		"testfile.go:11:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
	}

	f, fs := parse(code)
	issues := FmtErrorf(f)
	verifyIssues(t, fs, issues, expects)
}
