// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	// "reflect"
	// "strconv"
	// "strings"

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
		// START draft
		pos, ids := verifyEmptySlice(x)
		if len(ids) == 1 {
			var e = emptyslice{}
			issues = append(issues, &Issue{
				Pos:  fs.Position(pos),
				Msg:  fmt.Sprintf("Use 'var' statement when you declare empty slice '%s'", ids[0]),
				Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
				Fix:  &e,
			})
		} else if len(ids) >= 1 {
			var str string
			for i, id := range ids {
				str = str + "'" + id + "'"
				if i == len(ids)-2 {
					str = str + " and "
				} else if i != len(ids)-1 {
					str = str + ", "
				}
			}
			var e = emptyslice{Exp: "multiple erros"}
			issues = append(issues, &Issue{
				Pos:  fs.Position(pos),
				Msg:  fmt.Sprintf("Use 'var' statement when you declare empty slice %s", str),
				Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
				Fix:  &e,
			})
		}
		return true
	})

	return issues
}

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

// emptyslice is struct for interface AutoFix
type emptyslice struct {
	Exp string // nil or something to explain
}

// AutoFix automatically fix given issue and rewrite the file
func (e *emptyslice) AutoFix(i *Issue) {
	// create new fileset
	fs := token.NewFileSet()
	// get filename from issue
	filename := i.Pos.Filename
	// parse file and get ast.File
	astf, err := parser.ParseFile(fs, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatalln("Error:", err)
	}
	// get token.File using ast.File
	f := fs.File(astf.Pos())

	// Calculate correct offset number of issue using line and column number and convert it to token.Pos.
	offset, ok := findOffset(filename, i.Pos.Line, i.Pos.Column)
	if !ok {
		fmt.Errorf("offset of issue \"%s\" cannot be calculated", i.Msg)
	}
	pos := f.Pos(offset)

	// astutil.PathEnclosingInterval returns the node at pos and its ancestors.
	// path has node-tree to issue position
	path, _ := astutil.PathEnclosingInterval(astf, pos, pos)

	// TODO():  modularize
	id, _ := path[0].(*ast.Ident)
	ass, _ := path[1].(*ast.AssignStmt)
	// find index of identifier in Lhs
	var lindex int
	for i, le := range ass.Lhs {
		if le != id {
			continue
		}
		lindex = i
	}
	// corresponding CompositeLit is at lindex of Rhs
	comp, _ := ass.Rhs[lindex].(*ast.CompositeLit)
	compType, _ := comp.Type.(*ast.ArrayType)
	elt := compType.Elt
	block, _ := path[2].(*ast.BlockStmt)

	// identify which element in block need to be replaced with DeclStmt
	// (where AssignStmt is)
	var bindex int
	for i, stmt := range block.List {
		if assstmt, ok := stmt.(*ast.AssignStmt); !ok || assstmt != ass {
			continue
		}
		bindex = i
		break
	}

	// if there are multiple empty slice definition
	m := make(map[ast.Expr][]*ast.Ident)
	m[elt] = append(m[elt], id)
	mOrder := []ast.Expr{elt}

	var removeids = []int{lindex}
	if e.Exp != "" {
		for i, rh := range ass.Rhs[lindex+1:] {
			c, ok := rh.(*ast.CompositeLit)
			if !ok {
				continue
			}
			ct, ok := c.Type.(*ast.ArrayType)
			if !ok {
				continue
			}
			el := ct.Elt
			asterisks, starnode := toQualifiedStarExpr(el)
			elTypeStr := asterisks + toQualifiedName(starnode)
			newid, _ := ass.Lhs[lindex+1+i].(*ast.Ident)
			var newtype = true
			for key := range m {
				keyAsterisks, keyStarnode := toQualifiedStarExpr(key)
				keyTypeStr := keyAsterisks + toQualifiedName(keyStarnode)
				if elTypeStr == keyTypeStr {
					m[key] = append(m[key], newid)
					newtype = false
					break
				}
			}
			if newtype {
				m[el] = append(m[el], newid)
				mOrder = append(mOrder, el)
			}

			// store indices of elements should be removed from Lhs and Rhs
			removeids = append(removeids, lindex+1+i)
		}

		var corrects []*ast.DeclStmt
		for _, elm := range mOrder {
			corrects = append(corrects, &ast.DeclStmt{
				Decl: &ast.GenDecl{
					Tok:    token.VAR,
					TokPos: m[elm][0].NamePos,
					Specs: []ast.Spec{
						&ast.ValueSpec{
							Names: m[elm],
							Type: &ast.ArrayType{
								Elt: elm,
							},
						},
					},
				},
			})
		}

		k := len(m)
		if len(removeids) == len(ass.Lhs) { // if ass is all empty slice declaration
			block.List = append(block.List[:bindex+2], block.List[bindex+3-k:]...)
			for i := 0; i < k; i++ {
				block.List[bindex+i] = corrects[i]
			}
		} else {
			// remove invalid statement from multiple definition
			ass.Lhs = removeExprElement(ass.Lhs, removeids)
			ass.Rhs = removeExprElement(ass.Rhs, removeids)
			// expand blick list for k elements will be added
			block.List = append(block.List[:bindex+2], block.List[bindex+2-k:]...)
			remid, _ := ass.Lhs[0].(*ast.Ident)
			for i := 0; i < k; i++ {
				if takePositionOfDeclStmt(corrects[i]) < remid.NamePos {
					block.List[bindex+i+1] = block.List[bindex+i]
					block.List[bindex+i] = corrects[i]
				} else {
					block.List[bindex+i+1] = corrects[i]
				}
			}
		}
	} else {
		// make correct statement with id and elt
		correct := &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok:    token.VAR,
				TokPos: id.NamePos,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{
							id,
						},
						Type: &ast.ArrayType{
							Elt: elt,
						},
					},
				},
			},
		}

		if len(ass.Lhs) != 1 { // if it was a multiple assign statement
			// remove invalid statement from multiple definition
			ass.Lhs = append(ass.Lhs[:lindex], ass.Lhs[lindex+1:]...)
			ass.Rhs = append(ass.Rhs[:lindex], ass.Rhs[lindex+1:]...)
			if lindex == 0 {
				// add correct statement *before* the past statement
				block.List = append(block.List[:bindex+2], block.List[bindex+1:]...)
				block.List[bindex+1] = block.List[bindex]
				block.List[bindex] = correct
			} else {
				// add correct statement *below* the past statement
				block.List = append(block.List[:bindex+2], block.List[bindex+1:]...)
				block.List[bindex+1] = correct
			}
		} else { // if it was a single assign statement
			// simply replace the element with a corrected node
			block.List[bindex] = correct
		}
	}

	// rewrite the file
	file, _ := os.Create(filename)
	if err := format.Node(file, fs, astf); err != nil {
		log.Fatalln("Error:", err)
	}

}

// findOffset returns offset value that was calculated by line and column number.
func findOffset(filename string, line, column int) (int, bool) {
	dat, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalln("Error:", err)
	}
	fileText := string(dat)
	currentCol := 1
	currentLine := 1
	for offset, ch := range fileText {
		if currentLine == line && currentCol == column {
			return offset, true
		}
		if ch == '\n' {
			currentLine++
			currentCol = 1
		} else {
			currentCol++
		}
	}
	return -1, false
}

// toQualifiedStarExpr do something.
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

// removeExprElement do something.
func removeExprElement(exprs []ast.Expr, indices []int) []ast.Expr {
	for i := len(indices) - 1; i >= 0; i-- {
		exprs = append(exprs[:indices[i]], exprs[indices[i]+1:]...)
	}
	return exprs
}

// takePositionOfDeclStmt do something.
func takePositionOfDeclStmt(dest *ast.DeclStmt) token.Pos {
	gen, ok := dest.Decl.(*ast.GenDecl)
	if !ok {
		fmt.Println("the node doesn't have GenDecl as Decl")
		return token.NoPos
	}
	return gen.TokPos
}
