// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// VerifyCallingTHelper checks if unit test helper functions calling testing.T.Helper inside them.
func VerifyCallingTHelper(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	for _, decl := range f.Decls {
		if !isHelperTestFunc(decl) || !hasTestingT(decl) {
			continue
		}
		fn := decl.(*ast.FuncDecl)
		callTHelper := false
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if toQualifiedName(call.Fun) == "t.Helper" {
				callTHelper = true
				return false
			}
			return true
		})
		if !callTHelper {
			funcPos := fn.Name.Pos()
			funcName := fn.Name.Name
			issues = append(issues, &Issue{
				Pos:  fs.Position(funcPos),
				Msg:  fmt.Sprintf("testing.T.Helper should be called inside the helper function %s()", funcName), // name of the test
				Link: "https://golang.org/pkg/testing/#T.Helper",
			})
		}
	}
	return issues
}

func isHelperTestFunc(node ast.Node) bool {
	fn, ok := node.(*ast.FuncDecl)
	if !ok {
		return false
	}
	if strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	return true
}

func hasTestingT(node ast.Node) bool {
	fn, ok := node.(*ast.FuncDecl)
	if !ok {
		return false
	}
	foundTestingT := false
	for _, param := range fn.Type.Params.List {
		st, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if _, ok := st.X.(*ast.SelectorExpr); !ok {
			continue
		}
		if toQualifiedName(st.X) == "testing.T" {
			foundTestingT = true
		}
	}
	return foundTestingT
}
