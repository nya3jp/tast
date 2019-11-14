// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"log"
	"os"

	"golang.org/x/tools/go/ast/astutil"
)

// EmptySlice warns the invalid empty slice declaration.
func EmptySlice(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	var issues []*Issue

	// Traverse a syntax tree and find not-preferred empty slice declarations.
	newf := astutil.Apply(f, func(c *astutil.Cursor) bool {
		// If parent is not a block statement, ignore.
		_, ok := c.Parent().(*ast.BlockStmt)
		if !ok {
			return true
		}
		asgn, ok := c.Node().(*ast.AssignStmt)
		if !ok {
			return true
		}
		if asgn.Tok != token.DEFINE {
			return true
		}
		// Find issues and make map of identifier and empty slice.
		var remainids []int
		var ids []string
		var lrmap map[ast.Expr][]*ast.Ident = map[ast.Expr][]*ast.Ident{}
		var mOrder []ast.Expr
		var pos token.Pos
		for i, rexp := range asgn.Rhs {
			remainids = append(remainids, i)
			comp, ok := rexp.(*ast.CompositeLit)
			if !ok {
				continue
			}
			arr, ok := comp.Type.(*ast.ArrayType)
			if !ok {
				continue
			}
			if arr.Len == nil && comp.Elts == nil {
				id, _ := asgn.Lhs[i].(*ast.Ident) // confirmed that assignment operator is DEFINE
				ids = append(ids, id.Name)
				elt := arr.Elt
				var newtype = true
				for key := range lrmap {
					if typeNameString(elt) == typeNameString(key) {
						lrmap[key] = append(lrmap[key], id)
						newtype = false
						break
					}
				}
				if newtype {
					lrmap[elt] = append(lrmap[elt], id)
					mOrder = append(mOrder, elt)
				}
				// Store the position of first element.
				if pos == token.NoPos {
					pos = id.Pos()
				}
				// Remove index of element should be removed from Lhs and Rhs.
				remainids = remainids[:len(remainids)-1]
			}
		}
		// If fix is false, simply append issues. If fix is true, modify tree.
		if !fix {
			var str string
			if len(ids) == 1 {
				str = "'" + ids[0] + "'"
			} else if len(ids) >= 1 {
				for i, id := range ids {
					str = str + "'" + id + "'"
					if i == len(ids)-2 {
						str = str + " and "
					} else if i != len(ids)-1 {
						str = str + ", "
					}
				}
			}
			if str == "" {
				return true
			}
			issues = append(issues, &Issue{
				Pos:  fs.Position(pos),
				Msg:  fmt.Sprintf("use 'var' statement when you declare empty slice %s", str),
				Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
			})
		} else {
			// corrects holds corrected empty slice definition.
			var corrects []*ast.DeclStmt
			for _, elt := range mOrder {
				corrects = append(corrects, &ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok:    token.VAR,
						TokPos: lrmap[elt][0].NamePos,
						Specs: []ast.Spec{
							&ast.ValueSpec{
								Names: lrmap[elt],
								Type: &ast.ArrayType{
									Elt: elt,
								},
							},
						},
					},
				})
			}
			if len(remainids) == 0 { // if asgn consists of only empty slice declarations
				c.Replace(corrects[0])
				for i := 1; i < len(corrects); i++ {
					c.InsertAfter(corrects[i])
				}
			} else {
				// remid is first remaining identifier of the original statement.
				remid, ok := asgn.Lhs[remainids[0]].(*ast.Ident)
				if !ok {
					return true
				}
				var lhs []ast.Expr
				var rhs []ast.Expr
				for _, i := range remainids {
					lhs = append(lhs, asgn.Lhs[i])
					rhs = append(rhs, asgn.Rhs[i])
				}
				c.Replace(&ast.AssignStmt{
					Lhs: lhs,
					Tok: token.DEFINE,
					Rhs: rhs,
				})
				// Insert corrected node before or after the original statement based on its position.
				for _, correct := range corrects {
					if takePositionOfGenDeclStmt(correct) < remid.NamePos {
						c.InsertBefore(correct)
					} else {
						c.InsertAfter(correct)
					}
				}
			}
		}
		return true
	}, nil)

	// Format modified tree.
	if fix {
		filename := fs.Position(f.Pos()).Filename
		file, err := os.Create(filename)
		if err != nil {
			log.Fatalln("Error:", err)
			return issues
		}
		if err := format.Node(file, fs, newf); err != nil {
			log.Fatalln("Error:", err)
			return issues
		}
		if err := file.Close(); err != nil {
			log.Fatalln("Error:", err)
			return issues
		}
	}

	return issues
}

// typeNameString returns type name as string value with stars.
func typeNameString(node ast.Expr) string {
	asterisks, basenode := toQualifiedStarExpr(node)
	return asterisks + toQualifiedName(basenode)
}

// toQualifiedStarExpr returns asterisks as string value and a base node of type expression.
func toQualifiedStarExpr(node ast.Expr) (string, ast.Expr) {
	var asterisks string
	star, ok := node.(*ast.StarExpr)
	for ok {
		node = star.X
		star, ok = node.(*ast.StarExpr)
		asterisks = asterisks + "*"
	}
	return asterisks, node
}

// takePositionOfGenDeclStmt returns token position of generic declaration in given declaration statement.
func takePositionOfGenDeclStmt(dest *ast.DeclStmt) token.Pos {
	gen, ok := dest.Decl.(*ast.GenDecl)
	if !ok {
		fmt.Println("the node doesn't have GenDecl as Decl")
		return token.NoPos
	}
	return gen.TokPos
}
