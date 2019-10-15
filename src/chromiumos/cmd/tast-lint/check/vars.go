// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

const secretVarsLink = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/features/secret_vars.md#Naming-convention"

var varNameRE = regexp.MustCompile(`^\s*([\w\.]*)\s*:`)
var validVarNameRE = regexp.MustCompile(`^(\w+(?:\.\w+)?)\.[a-zA-Z]\w*$`)

// VarFile checks variables defined in yaml format conforms with our convention.
func VarFile(path string, data []byte) []*Issue {
	var issues []*Issue
	for i, line := range strings.Split(string(data), "\n") {
		m := varNameRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]

		m = validVarNameRE.FindStringSubmatch(name)
		if m == nil {
			issues = append(issues, &Issue{
				Pos: token.Position{
					Filename: path,
					Line:     i + 1,
					Column:   1,
				},
				Msg:  fmt.Sprintf(`variable name %q; should have the form of "foo.Bar.X" or "foo.X" where X matches [a-zA-Z]\w*`, name),
				Link: secretVarsLink,
			})
			continue
		}

		if wantBase := m[1] + ".yaml"; filepath.Base(path) != wantBase {
			issues = append(issues, &Issue{
				Pos: token.Position{
					Filename: path,
					Line:     i + 1,
					Column:   1,
				},
				Msg:  fmt.Sprintf(`variable %q should be defined in file %q`, name, wantBase),
				Link: secretVarsLink,
			})
		}
	}
	return issues
}
