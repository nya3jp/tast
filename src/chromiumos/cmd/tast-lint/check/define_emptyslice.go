// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// EmptySlice warns the invalid empty slice declaration.
func EmptySlice(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	ast.Inspect(f, func(node ast.Node) bool {
		x, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if x.Tok != token.DEFINE {
			return true
		}
		for i, rexp := range x.Rhs {
			lvar, ok := x.Lhs[i].(*ast.Ident)
			if !ok {
				// If assignment operator is DEFINE, it is expected that each lhs is an identifier,
				// so this shouldn't happen. Skip to avoid false positive.
				continue
			}
			str := lvar.Name
			comp, ok := rexp.(*ast.CompositeLit)
			if !ok {
				continue
			}
			arr, ok := comp.Type.(*ast.ArrayType)
			if !ok {
				continue
			}
			if arr.Len == nil && comp.Elts == nil {
				issues = append(issues, &Issue{
					Pos:  fs.Position(lvar.Pos()),
					Msg:  fmt.Sprintf("Use 'var' statement when you declare empty slice '%s'", str),
					Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
				})
			}
		}
		return true
	})

	return issues
}
