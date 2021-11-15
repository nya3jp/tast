// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast/testutil"
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

func verifyAutoFix(t *testing.T, lintfunc func(*token.FileSet, *ast.File, bool) []*Issue, files, expects map[string]string) {
	t.Helper()

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)
	testutil.WriteFiles(tempDir, files)

	fs := token.NewFileSet()
	for filename := range files {
		path := filepath.Join(tempDir, filename)
		f, err := parser.ParseFile(fs, path, nil, parser.ParseComments)
		if err != nil {
			t.Error(err)
			continue
		}

		lintfunc(fs, f, true)

		if err := func() error {
			file, err := os.Create(path)
			if err != nil {
				return err
			}
			defer file.Close()
			if err := format.Node(file, fs, f); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			t.Error(err)
			continue
		}

		bytesread, err := ioutil.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		if err := fmtError(bytesread); err != nil {
			t.Error(err)
			continue
		}
	}

	files, err := testutil.ReadFiles(tempDir)
	if err != nil {
		t.Error(err)
	}

	for filename := range files {
		got := splitLines(files[filename])
		want := splitLines(expects[filename])

		if len(got) > len(want) {
			for i := range want {
				if diff := cmp.Diff(got[i], want[i], cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("AutoFix failed (-got +want):%s:%d:\n%s", filename, i+1, diff)
				}
			}
			for i := len(want); i < len(got); i++ {
				if diff := cmp.Diff(got[i], "", cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("AutoFix failed (-got +want):%s:%d:\n%s", filename, i+1, diff)
				}
			}
		} else {
			for i := range got {
				if diff := cmp.Diff(got[i], want[i], cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("AutoFix failed (-got +want):%s:%d:\n%s", filename, i+1, diff)
				}
			}
			for i := len(got); i < len(want); i++ {
				if diff := cmp.Diff("", want[i], cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("AutoFix failed (-got +want):%s:%d:\n%s", filename, i+1, diff)
				}
			}
		}
	}
}

// splitLines split given string into string slice of lines which was ended with "\n".
// Also, all "\t" are replaced by a white space.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.Replace(lines[i], "\t", " ", -1)
	}
	return lines
}

// fmtError runs gofmt to see if code has any formatting error.
func fmtError(code []byte) error {
	cmd := exec.Command("gofmt", "-l")
	cmd.Stdin = bytes.NewBuffer(code)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Failed gofmt: %v", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("File's formatting is different from gofmt's: %s", out)
	}
	return nil
}
