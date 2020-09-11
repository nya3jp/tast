// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

// ForbiddenImports makes sure blocked errors packages are not imported.
func ForbiddenImports(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		if p == "errors" || p == "github.com/pkg/errors" {
			issues = append(issues, &Issue{
				Pos:  fs.Position(im.Pos()),
				Msg:  fmt.Sprintf("chromiumos/tast/errors package should be used instead of %s package", p),
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
			})
		}
	}
	return issues
}
