// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strconv"
)

// VerifyInformationalAttr checks if a newly added test has 'informational' attribute.
func VerifyInformationalAttr(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "init" {
			// Not an init() function declaration. Skip.
			continue
		}
		for _, stmt := range fn.Body.List {
			estmt, ok := stmt.(*ast.ExprStmt)
			if !ok || !isTestingAddTestCall(estmt.X) {
				continue
			}

			call := estmt.X.(*ast.CallExpr)
			if len(call.Args) != 1 {
				// This should be checked by a compiler, so skipped.
				continue
			}

			// Verify the argument is "&testing.Test{...}"
			arg, ok := call.Args[0].(*ast.UnaryExpr)
			if !ok || arg.Op != token.AND {
				continue
			}
			comp, ok := arg.X.(*ast.CompositeLit)
			if !ok {
				continue
			}

			for _, el := range comp.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				ident, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				if ident.Name == "Attr" {
					comp, ok := kv.Value.(*ast.CompositeLit)
					if !ok {
						continue
					}
					hasInformational := false
					for _, el := range comp.Elts {
						lit, ok := el.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						s, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						if s == "informational" {
							hasInformational = true
							break
						}
					}
					if hasInformational == false {
						issues = append(issues, &Issue{
							Pos:  fs.Position(el.Pos()),
							Msg:  "Newly added tests should be marked as 'informational'.",
							Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-registration",
						})
					}
				}
			}
		}
	}

	return issues
}
