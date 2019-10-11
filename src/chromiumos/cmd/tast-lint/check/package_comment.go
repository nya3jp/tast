// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// PackageComment checks if there is a document for package of given files
func PackageComment(fs *token.FileSet, dfmap map[string][]*ast.File) []*Issue {
	var issues []*Issue

	for _, v := range dfmap {
		hasNoDoc := true
		var packageName string
		var packagePos token.Pos
		for _, f := range v {
			if f.Doc != nil {
				hasNoDoc = false
				break
			}
			packageName = f.Name.Name
			packagePos = f.Package
		}
		if hasNoDoc {
			issues = append(issues, &Issue{
				Pos:  fs.Position(packagePos),
				Msg:  fmt.Sprintf("document of newly created package '%s' is required in one of the files in this directory", packageName),
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Documentation",
			})
		}
	}

	return issues
}
