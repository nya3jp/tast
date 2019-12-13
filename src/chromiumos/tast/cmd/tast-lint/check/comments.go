// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// Comments checks comments on unexported functions (if any) start with the function name.
// Golint examines only exported functions. This check is a complementary of it.
func Comments(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		fn, ok := node.(*ast.FuncDecl)
		// Exported functions are checked by golint.go .
		if !ok || ast.IsExported(fn.Name.Name) {
			return
		}
		doc := fn.Doc
		if doc == nil {
			return
		}
		prefix := fn.Name.Name + " "
		if strings.HasPrefix(doc.Text(), prefix) {
			return
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(doc.Pos()),
			Msg:  fmt.Sprintf("Comment on function %s should be of the form %q", fn.Name.Name, prefix+"..."),
			Link: "https://github.com/golang/go/wiki/CodeReviewComments#comment-sentences",
		})
	})

	ast.Walk(v, f)
	return issues
}
