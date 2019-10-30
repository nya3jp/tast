// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"

	"golang.org/x/tools/go/ast/astutil"
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
		pos, ids := verifyEmptySlice(x)
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
		} else {
			return true
		}
		var e = emptyslice{}
		issues = append(issues, &Issue{
			Pos:  fs.Position(pos),
			Msg:  fmt.Sprintf("use 'var' statement when you declare empty slice %s", str),
			Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
			Fix:  &e,
		})
		return true
	})

	return issues
}

// verifyEmptySlice returns position and name of identifiers which are empty-slices in given assign statement.
func verifyEmptySlice(x *ast.AssignStmt) (token.Pos, []string) {
	var pos token.Pos
	var ids []string
	for i, rexp := range x.Rhs {
		comp, ok := rexp.(*ast.CompositeLit)
		if !ok {
			continue
		}
		arr, ok := comp.Type.(*ast.ArrayType)
		if !ok {
			continue
		}
		if arr.Len == nil && comp.Elts == nil {
			id, _ := x.Lhs[i].(*ast.Ident) // confirmed that assignment operator is DEFINE
			ids = append(ids, id.Name)
			if pos == token.NoPos {
				pos = id.Pos() // store the position of first element
			}
		}
	}
	return pos, ids
}

// emptyslice is struct for interface AutoFix.
type emptyslice struct{}

// AutoFix automatically fix given issue and rewrite the file.
func (e *emptyslice) AutoFix(i *Issue) {
	filename, fs, tokenf, astf := fileInfoForAutoFix(i)
	// Modify AST.
	if err := modifyAST(astf, tokenf.Pos(i.Pos.Offset)); err != nil {
		log.Fatalln("Error:", err)
	}
	// Rewrite the file.
	file, _ := os.Create(filename)
	if err := format.Node(file, fs, astf); err != nil {
		log.Fatalln("Error:", err)
	}
}

// fileInfoForAutoFix returns name, fileset, a file and parsed file.
func fileInfoForAutoFix(i *Issue) (string, *token.FileSet, *token.File, *ast.File) {
	// get filename from issue
	filename := i.Pos.Filename
	// create new fileset
	fs := token.NewFileSet()
	// parse file and get ast.File
	astf, err := parser.ParseFile(fs, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatalln("Error:", err)
	}
	// Get token.File using ast.File.
	tokenf := fs.File(astf.Pos())

	return filename, fs, tokenf, astf
}

// modifyAST modify given AST node at given position of issue.
func modifyAST(astf *ast.File, pos token.Pos) error {
	// astutil.PathEnclosingInterval returns the node at pos and its ancestors.
	// path has node-tree to issue position. path[0] is first empty slice identifier node.
	path, exact := astutil.PathEnclosingInterval(astf, pos, pos)
	if !exact {
		return errors.New("cannot get ancestors of given node")
	}
	// asgn has invalid empty slice declaration(s).
	asgn, ok := path[1].(*ast.AssignStmt)
	if !ok {
		return errors.New("parent node of identifier is not assignment statement")
	}

	// Make map m that is from empty slice expression to corresponding identifier node(s).
	m := make(map[ast.Expr][]*ast.Ident)
	// mOrder keeps order of map m.
	var mOrder []ast.Expr
	var removeids []int
	for i := 0; i < len(asgn.Rhs); i++ {
		elt, err := takeEltOfEmptySlice(asgn, i)
		if err != nil {
			continue
		}
		id, ok := asgn.Lhs[i].(*ast.Ident)
		if !ok {
			continue
		}
		var newtype = true
		for key := range m {
			if typeNameString(elt) == typeNameString(key) {
				m[key] = append(m[key], id)
				newtype = false
				break
			}
		}
		if newtype {
			m[elt] = append(m[elt], id)
			mOrder = append(mOrder, elt)
		}
		// Store indices of elements should be removed from Lhs and Rhs.
		removeids = append(removeids, i)
	}

	// corrects holds corrected empty slice definition.
	var corrects []*ast.DeclStmt
	for _, elt := range mOrder {
		corrects = append(corrects, &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok:    token.VAR,
				TokPos: m[elt][0].NamePos,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: m[elt],
						Type: &ast.ArrayType{
							Elt: elt,
						},
					},
				},
			},
		})
	}

	// block has asgn.
	block, ok := path[2].(*ast.BlockStmt)
	if !ok {
		return errors.New("assignment statement is not in block statement")
	}
	// Find index of assignment statement in block.
	var bindex int
	for i, stmt := range block.List {
		if stmt != asgn {
			continue
		}
		bindex = i
		break
	}
	k := len(m)                          // number of lines of corrected empty slice declaration
	if len(removeids) == len(asgn.Lhs) { // if asgn consists of only empty slice declarations
		block.List = append(block.List[:bindex+2], block.List[bindex+3-k:]...)
		for i := 0; i < k; i++ {
			block.List[bindex+i] = corrects[i]
		}
	} else {
		// Remove invalid empty slice statements from asgn.
		asgn.Lhs = removeExprElement(asgn.Lhs, removeids)
		asgn.Rhs = removeExprElement(asgn.Rhs, removeids)
		// Expand block list for k elements will be added.
		block.List = append(block.List[:bindex+2], block.List[bindex+2-k:]...)
		// remid is remaining identifier of the original statement.
		remid, ok := asgn.Lhs[0].(*ast.Ident)
		if !ok {
			return errors.New("invalid node")
		}
		// Insert corrected node before or after the original statement based on its position.
		for i := 0; i < k; i++ {
			if takePositionOfGenDeclStmt(corrects[i]) < remid.NamePos {
				block.List[bindex+i+1] = block.List[bindex+i]
				block.List[bindex+i] = corrects[i]
			} else {
				block.List[bindex+i+1] = corrects[i]
			}
		}
	}

	return nil
}

// takeEltOfEmptySlice returns expression node of empty slice.
func takeEltOfEmptySlice(asgn *ast.AssignStmt, index int) (ast.Expr, error) {
	comp, ok := asgn.Rhs[index].(*ast.CompositeLit)
	if !ok {
		return nil, errors.New("invalid node")
	}
	compType, ok := comp.Type.(*ast.ArrayType)
	if !ok {
		return nil, errors.New("invalid node")
	}
	return compType.Elt, nil
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

// removeExprElement removes elements at given indices from expression slice.
func removeExprElement(exprs []ast.Expr, indices []int) []ast.Expr {
	for i := len(indices) - 1; i >= 0; i-- {
		exprs = append(exprs[:indices[i]], exprs[indices[i]+1:]...)
	}
	return exprs
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
