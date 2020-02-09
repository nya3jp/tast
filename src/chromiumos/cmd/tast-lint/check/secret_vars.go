// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// varFileBaseNameRE matches with a valid variable file basename, e.g. foo.Bar.yaml or foo.yaml.
var varFileBaseNameRE = regexp.MustCompile(`^([a-z][a-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)?\.)yaml$`)

// varLastPartRE matches with a valid last part (after the last dot) of variable names.
var varLastPartRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

const secretVarNamingURL = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#secret-variables`

// SecretVarFile checks that files defining variables follow our convention.
// TODO(oka): Consider checking the file is formatted with a formatter.
func SecretVarFile(path string, data []byte) (issues []*Issue) {
	addIssue := func(msg string) {
		issues = append(issues, &Issue{
			Pos:  token.Position{Filename: path},
			Msg:  msg,
			Link: secretVarNamingURL,
		})
	}

	// Check filename.
	base := filepath.Base(path)
	var wantPrefix string // "foo.Bar." or "foo."
	if m := varFileBaseNameRE.FindStringSubmatch(base); m == nil {
		addIssue(fmt.Sprint("File's basename doesn't match expected regex ", varFileBaseNameRE))
	} else {
		wantPrefix = m[1]
	}

	// Check contents.
	vars := make(map[string]string)
	if err := yaml.Unmarshal(data, vars); err != nil {
		addIssue(fmt.Sprint("Failed to parse the file data as key value pairs: ", err.Error()))
	}

	if len(issues) > 0 {
		return
	}

	for varName := range vars {
		switch {
		case !strings.HasPrefix(varName, wantPrefix):
			fallthrough
		case !varLastPartRE.MatchString(strings.TrimPrefix(varName, wantPrefix)):
			addIssue(fmt.Sprintf("Var name %s violates naming convention; it should match %s[A-Za-z][A-Za-z0-9_]*", varName, wantPrefix))
		}
	}
	return
}
