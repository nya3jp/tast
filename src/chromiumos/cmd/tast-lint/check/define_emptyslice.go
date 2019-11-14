// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// EmptySlice warns the invalid empty slice declaration.
func EmptySlice(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	var issues []*Issue

	// Traverse a syntax tree and find not-preferred empty slice declarations.
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		// If parent is not a block statement, ignore.
		// - For example, empty slice assginment in the statement like
		//     `if a := []int{}; len(a) == 1 { // parent is ast.IfStmt
		//        return
		//     }`
		//   is not replacable by 'var' statement.
		if _, ok := c.Parent().(*ast.BlockStmt); !ok {
			return true
		}

		asgn, ok := c.Node().(*ast.AssignStmt)
		if !ok {
			return true
		}
		if asgn.Tok != token.DEFINE {
			return true
		}

		// Find invalid empty slice declarations.
		var ids []*ast.Ident
		var elt ast.Expr

		for i, rexp := range asgn.Rhs {
			comp, ok := rexp.(*ast.CompositeLit)
			if !ok {
				continue
			}

			arr, ok := comp.Type.(*ast.ArrayType)
			if !ok {
				continue
			}

			if arr.Len == nil && comp.Elts == nil {
				id, ok := asgn.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}

				ids = append(ids, id)
				elt = arr.Elt
			}
		}

		if len(ids) == 0 {
			return true
		}

		fixable := len(asgn.Rhs) == 1
		if !fix {
			var idNames []string
			for _, id := range ids {
				idNames = append(idNames, id.Name)
			}
			issue := &Issue{
				Pos:     fs.Position(asgn.Pos()),
				Msg:     fmt.Sprintf("Use 'var' statement when you declare empty slice(s): %s", strings.Join(idNames, ", ")),
				Link:    "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
				Fixable: fixable,
			}
			issues = append(issues, issue)
		} else if fix && fixable {
			c.Replace(&ast.DeclStmt{
				Decl: &ast.GenDecl{
					Tok:    token.VAR,
					TokPos: ids[0].NamePos,
					Specs: []ast.Spec{
						&ast.ValueSpec{
							Names: ids,
							Type: &ast.ArrayType{
								Elt: elt,
							},
						},
					},
				},
			})
		}

		return true
	}, nil)

	return issues
}
