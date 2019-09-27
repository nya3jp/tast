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
		switch x := node.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				return true
			}
			lvar, ok := x.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			str := lvar.Name
			for _, rexp := range x.Rhs {
				comp, ok := rexp.(*ast.CompositeLit)
				if !ok {
					continue
				}
				arr, ok := comp.Type.(*ast.ArrayType)
				if !ok {
					continue
				}
				len := arr.Len
				els := comp.Elts
				if len == nil && els == nil {
					issues = append(issues, &Issue{
						Pos:  fs.Position(lvar.Pos()),
						Msg:  fmt.Sprintf("Use 'var' statement when you declare empty slice '%s'", str),
						Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
					})
				}
			}
		}
		return true
	})

	return issues
}
