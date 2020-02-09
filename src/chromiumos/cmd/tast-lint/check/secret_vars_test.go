// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"testing"
)

func TestTestLevelVarFile_OK(t *testing.T) {
	const (
		path = "vars/foo.Bar.yaml"
		data = `foo.Bar.baz: 1
foo.Bar.qux: a
foo.Bar.empty:`
	)

	var expects []string

	issues := SecretVarFile(path, []byte(data))
	verifyIssues(t, issues, expects)
}

func TestCategoryLevelVarFile_OK(t *testing.T) {
	const (
		path = "vars/foo.yaml"
		data = `foo.baz: 1
foo.qux: a
foo.empty:`
	)

	// The import order is good, so no issue.
	var expects []string
	issues := SecretVarFile(path, []byte(data))
	verifyIssues(t, issues, expects)
}

const invalidVarNameErrTmpl = "%s: Var name %s violates naming convention; it should match %s[A-Za-z][A-Za-z0-9_]*"

func TestTestLevelVarFile(t *testing.T) {
	const path = "vars/a.B.yaml"
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"a.B", false},
		{"a.B.", false},
		{"a.B.1", false},
		{"a.B.c", true},
		{"a.B.C", true},
		{"a.B._c", false},
		{"a.B.c.d", false},
		{"a.B..c", false},
		{"a.B.c_1xX", true},
		{"a.B.c$", false},
		{"a.Bc", false},
		{"a.Z.c", false},
		{"z.B.c", false},
		{".B.c", false},
		{"a..c", false},
	} {
		var expects []string
		if !tc.valid {
			expects = append(expects, fmt.Sprintf(invalidVarNameErrTmpl, path, tc.name, "a.B."))
		}
		issues := SecretVarFile(path, []byte(tc.name+":"))
		verifyIssues(t, issues, expects)
	}
}

func TestCategoryLevelVarFile(t *testing.T) {
	const path = "vars/a.yaml"

	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"a", false},
		{"a.", false},
		{"a.c", true},
		{"a._c", false},
		{"a.c.d", false},
		{"a..c", false},
		{"a.C_1x", true},
		{"a.$", false},
		{"ac", false},
		{"z.c", false},
	} {
		var expects []string
		if !tc.valid {
			expects = append(expects, fmt.Sprintf(invalidVarNameErrTmpl, path, tc.name, "a."))
		}
		issues := SecretVarFile(path, []byte(tc.name+":"))
		verifyIssues(t, issues, expects)
	}

}

func TestInvalidFileNames(t *testing.T) {
	paths := []string{
		".yaml",
		"a..yaml",
		"a.B.c.yaml",
		"_.yaml",
	}
	const data = ""

	for _, path := range paths {
		expects := []string{
			path + `: File's basename doesn't match expected regex ^([a-z][a-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)?\.)yaml$`,
		}
		issues := SecretVarFile(path, []byte(data))
		verifyIssues(t, issues, expects)
	}
}

func TestInvalidFileNameAndParseError(t *testing.T) {
	const (
		path = "vars/.yaml"
		data = "1"
	)

	expects := []string{
		path + `: File's basename doesn't match expected regex ^([a-z][a-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)?\.)yaml$`,
		path + ": Failed to parse the file data as key value pairs: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!int `1` into map[string]string",
	}
	issues := SecretVarFile(path, []byte(data))
	verifyIssues(t, issues, expects)
}
