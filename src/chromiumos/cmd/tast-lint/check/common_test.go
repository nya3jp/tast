// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bufio"
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

func verifyAutoFix(t *testing.T, lintfunc func(*token.FileSet, *ast.File, bool) []*Issue, files map[string]string, expects map[string]string) {
	t.Helper()

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)
	testutil.WriteFiles(tempDir, files)

	var issues []*Issue
	fs := token.NewFileSet()
	for filename := range files {
		f, err := parser.ParseFile(fs, filepath.Join(tempDir, filename), nil, parser.ParseComments)
		if err != nil {
			t.Error(err)
		}

		issues = append(issues, lintfunc(fs, f, true)...)

		path := fs.Position(f.Pos()).Filename
		if err := func() error {
			tempfile, err := ioutil.TempFile(filepath.Dir(path), "temp")
			defer os.Remove(tempfile.Name())
			defer tempfile.Close()
			if err != nil {
				return err
			}
			buf := bufio.NewWriter(tempfile)
			if err := format.Node(buf, fs, f); err != nil {
				return err
			}
			buf.Flush()
			bytesread, err := ioutil.ReadFile(tempfile.Name())
			if err != nil {
				return err
			}
			if err := fmtError(bytesread, tempfile.Name()); err != nil {
				return err
			}
			if err := os.Rename(tempfile.Name(), path); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			t.Error(err)
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
func fmtError(code []byte, path string) error {
	cmd := exec.Command("gofmt", "-l")
	cmd.Stdin = bytes.NewBuffer(code)
	out, err := cmd.Output()
	if err != nil || len(out) > 0 {
		return fmt.Errorf("Failed gofmt %s: %v", path, err)
	}
	return nil
}
