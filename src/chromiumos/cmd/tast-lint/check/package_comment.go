// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// PackageComment checks if there is a document for given package.
func PackageComment(fs *token.FileSet, pkg *ast.Package) []*Issue {
	hasDoc := false
	var packagePos token.Pos
	var packageName string
	for _, f := range pkg.Files {
		if f.Doc != nil {
			hasDoc = true
			break
		}
		if f.Package > packagePos { // to make sure the issue stores information of the last file
			packagePos = f.Package
			packageName = f.Name.Name
		}
	}
	var issues []*Issue
	if !hasDoc {
		issues = append(issues, &Issue{
			Pos:  fs.Position(packagePos),
			Msg:  fmt.Sprintf("document of newly created package '%s' is required in one of the files in this directory", packageName),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Documentation",
		})
	}
	return issues
}
