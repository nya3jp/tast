// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// PackageComment checks if there is a document for package of given file
func PackageComment(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	packageName := f.Name.Name
	if f.Doc == nil {
		issues = append(issues, &Issue{
			Pos:  fs.Position(f.Package),
			Msg:  fmt.Sprintf("document of newly created package '%s' is required", packageName),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Documentation",
		})
	}
	// Comment format is checked by golint.go .
	return issues
}
