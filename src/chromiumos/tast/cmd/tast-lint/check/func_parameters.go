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
	astutil.Apply(f, func(c *astutil.Cursor) (cont bool) {
		// Always continue traversal.
		cont = true

		n, ok := c.Node().(*ast.FuncType)
		if !ok {
			return
		}
		for _, l := range []*ast.FieldList{n.Params, n.Results} {
			if l == nil || l.List == nil || l.List[0].Names == nil {
				continue
			}
			var nfis []*ast.Field
			var names []*ast.Ident
			var prev *ast.Field
			var removable []ast.Expr
			for _, fi := range l.List {
				if prev == nil {
					names = append(names, fi.Names...)
					prev = fi
					continue
				}
				// This seems to be always false (i.e. all the fields in
				// non-nil prev are nil). We have it to be on the safe side.
				shouldSeparate := prev.Doc != nil || prev.Tag != nil || prev.Comment != nil
				if shouldSeparate || !sameType(fs, prev.Type, fi.Type) {
					nfis = append(nfis, &ast.Field{
						Doc:     prev.Doc,
						Names:   names,
						Type:    prev.Type,
						Tag:     prev.Tag,
						Comment: prev.Comment,
					})
					names = nil
				} else {
					removable = append(removable, prev.Type)
				}
				names = append(names, fi.Names...)
				prev = fi
			}
			nfis = append(nfis, &ast.Field{
				Doc:     prev.Doc,
				Names:   names,
				Type:    prev.Type,
				Tag:     prev.Tag,
				Comment: prev.Comment,
			})
			if fix {
				l.List = nfis
			} else {
				for _, t := range removable {
					issues = append(issues, &Issue{
						Pos:     fs.Position(t.Pos()),
						Msg:     "When two or more consecutive named function parameters share a type, you can omit the type from all but the last",
						Link:    "https://tour.golang.org/basics/5",
						Fixable: true,
					})
				}
			}
		}
		return
	}, nil)
	return
}

// sameType returns whether x and y have the same string representation.
// Both x and y should be representing a type.
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
