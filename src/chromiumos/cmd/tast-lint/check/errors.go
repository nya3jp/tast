// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"strconv"
)

// ErrorsImports makes sure blacklisted errors packages are not imported.
func ErrorsImports(f *ast.File) []*Issue {
	var issues []*Issue

	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		if p == "errors" || p == "github.com/pkg/errors" {
			issues = append(issues, &Issue{
				Pos: im.Pos(),
				Msg: fmt.Sprintf("chromiumos/tast/errors package should be used instead of %s package", p),
			})
		}
	}
	return issues
}

type funcVisitor func(node ast.Node)

func (v funcVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	v(node)
	return v
}

// FmtErrorf makes sure fmt.Errorf is not used.
func FmtErrorf(f *ast.File) []*Issue {
	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Errorf" {
			return
		}
		// TODO(nya): Support importing fmt with different aliases.
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != "fmt" {
			return
		}
		issues = append(issues, &Issue{
			Pos: x.Pos(),
			Msg: "chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
		})
	})

	for _, d := range f.Decls {
		ast.Walk(v, d)
	}
	return issues
}
