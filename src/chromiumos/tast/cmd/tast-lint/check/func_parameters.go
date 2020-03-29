// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// FuncParams checks function parameters and results.
func FuncParams(fs *token.FileSet, f *ast.File, fix bool) (issues []*Issue) {
	if fix {
		funcParamsAutofix(fs, f)
		return nil
	}
	astutil.Apply(f, func(c *astutil.Cursor) (cont bool) {
		// Always continue traversal.
		cont = true

		n, ok := c.Node().(*ast.FuncType)
		if !ok {
			return
		}
		for _, l := range []*ast.FieldList{n.Params, n.Results} {
			if l == nil {
				continue
			}
			var prev *ast.Field
			for _, fi := range l.List {
				if fi.Names == nil {
					break
				}
				if prev != nil && sameType(fs, prev.Type, fi.Type) {
					issues = append(issues, &Issue{
						Pos:     fs.Position(prev.Type.Pos()),
						Msg:     "When two or more consecutive named function parameters share a type, you can omit the type from all but the last",
						Link:    "https://tour.golang.org/basics/5",
						Fixable: true,
					})
				}
				prev = fi
			}
		}
		return
	}, nil)
	return
}

func funcParamsAutofix(fs *token.FileSet, f *ast.File) {
	astutil.Apply(f, func(c *astutil.Cursor) (cont bool) {
		// Always continue traversal.
		cont = true

		n, ok := c.Node().(*ast.FuncType)
		if !ok {
			return
		}
		for _, l := range []*ast.FieldList{n.Params, n.Results} {
			if l == nil {
				continue
			}
			var nfis []*ast.Field
			var names []*ast.Ident
			var prev *ast.Field
			for _, fi := range l.List {
				if fi.Names == nil {
					break
				}
				if prev != nil && !sameType(fs, prev.Type, fi.Type) {
					nfis = append(nfis, &ast.Field{
						Doc:     prev.Doc,
						Names:   names,
						Type:    prev.Type,
						Tag:     prev.Tag,
						Comment: prev.Comment,
					})
					names = nil
				}
				names = append(names, fi.Names...)
				prev = fi
			}
			if prev != nil {
				nfis = append(nfis, &ast.Field{
					Doc:     prev.Doc,
					Names:   names,
					Type:    prev.Type,
					Tag:     prev.Tag,
					Comment: prev.Comment,
				})
			}
			if nfis != nil {
				l.List = nfis
			}
		}
		return
	}, nil)
}

// sameType returns whether x and y are the same type.
func sameType(fs *token.FileSet, x, y ast.Expr) bool {
	var xb bytes.Buffer
	if err := format.Node(&xb, fs, x); err != nil {
		return false
	}
	var yb bytes.Buffer
	if err := format.Node(&yb, fs, y); err != nil {
		return false
	}
	return xb.String() == yb.String()
}
