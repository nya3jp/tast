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
	"reflect"
	"strconv"
	"strings"

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
				var e *emptyslice
				issues = append(issues, &Issue{
					Pos:  fs.Position(lvar.Pos()),
					Msg:  fmt.Sprintf("Use 'var' statement when you declare empty slice '%s'", str),
					Link: "https://github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices",
					Fix:  e,
				})
			}
		}
		return true
	})

	return issues
}

// emptyslice is struct for interface AutoFix
type emptyslice struct{}

// AutoFix automatically fix given issue and rewrite the file
func (*emptyslice) AutoFix(i *Issue) {
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

	// Store the line number of line breaks as int slice before the file will be modified
	// (this is necessary because when node is replaced with correct node, new line is added for some reason :( )
	vlines := reflect.ValueOf(f).Elem().FieldByName("lines")
	s := fmt.Sprint(vlines)
	s = strings.Trim(s, "[")
	s = strings.Trim(s, "]")
	var lines []int
	for _, e := range strings.Split(s, " ") {
		i, err := strconv.Atoi(e)
		if err != nil {
			continue
		}
		lines = append(lines, i) // lines will be used later
	}

	// Calculate correct offset number of issue using line and column number and convert it to token.Pos.
	offset, ok := findOffset(filename, i.Pos.Line, i.Pos.Column)
	if !ok {
		fmt.Errorf("offset of issue \"%s\" cannot be calculated", i.Msg)
	}
	pos := f.Pos(offset)

	// astutil.PathEnclosingInterval returns the node at pos and its ancestors.
	// path has node-tree to issue position
	path, _ := astutil.PathEnclosingInterval(astf, pos, pos)

	// TODO(): handle comment problem and modularize
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

	// make correct statement with id and elt
	correct := &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
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
		// add correct statement below the past statement
		block.List = append(block.List[:bindex+2], block.List[bindex+1:]...)
		block.List[bindex+1] = correct
		// remove invalid statement from multiple definition
		ass.Lhs = append(ass.Lhs[:lindex], ass.Lhs[lindex+1:]...)
		ass.Rhs = append(ass.Rhs[:lindex], ass.Rhs[lindex+1:]...)
	} else { // if it was a single assign statement
		// simply replace the element with a corrected node
		block.List[bindex] = correct
	}

	// rewrite the file
	file, _ := os.Create(filename)
	if err := format.Node(file, fs, astf); err != nil {
		log.Fatalln("Error:", err)
	}

	// newline removal (looks redundant but go well...)
	// parse again
	astf, err = parser.ParseFile(fs, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatalln("Error:", err)
	}
	// remove the line break that was added when the node was replaced
	f.SetLines(lines)
	// rewrite the file again
	file, _ = os.Create(filename)
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
