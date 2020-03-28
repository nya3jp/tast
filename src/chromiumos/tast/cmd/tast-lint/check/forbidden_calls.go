// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// ForbiddenCalls checks if any forbidden functions are called.
func ForbiddenCalls(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	isUnitTest := isUnitTestFile(fs.Position(f.Package).Filename)
	var issues []*Issue
	var errorsAdded bool

	// Fixable condition is defined as (1) the file don't have the ident node
	// whose name is "errors" and (2) the file is able to be ran goimports.
	// TODO: handle the case if there is "chromium/tast/errors" in import list.
	fixable := false
	if !hasErrorsIdent(f) {
		if src, err := formatASTNode(fs, f); err == nil {
			fixable = goimportApplicable(src)
		}
	}

	astutil.Apply(f, func(c *astutil.Cursor) bool {
		sel, ok := c.Node().(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// TODO(nya): Support imports with different aliases.
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		call := fmt.Sprintf("%s.%s", x.Name, sel.Sel.Name)
		switch call {
		case "context.Background", "context.TODO":
			if !isUnitTest {
				issues = append(issues, &Issue{
					Pos:  fs.Position(x.Pos()),
					Msg:  call + " ignores test timeout; use test function's ctx arg instead",
					Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
				})
			}
		case "fmt.Errorf":
			if !fix {
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     "chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
					Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
					Fixable: fixable,
				})
			} else if fixable {
				c.Replace(&ast.SelectorExpr{
					X: &ast.Ident{
						Name: "errors",
					},
					Sel: &ast.Ident{
						Name: "Errorf",
					},
				})
				errorsAdded = true
			}
		case "time.Sleep":
			issues = append(issues, &Issue{
				Pos:  fs.Position(x.Pos()),
				Msg:  "time.Sleep ignores context deadline; use testing.Poll or testing.Sleep instead",
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
			})
		}

		return true
	}, nil)

	if errorsAdded {
		if astutil.AddImport(fs, f, "chromiumos/tast/errors") {
			newf, err := ImportOrderAutoFix(fs, f)
			if err != nil {
				return issues
			}
			*f = *newf
		}
	}

	return issues
}

// hasErrorsIdent returns true if there is an identifier node whose name is "errors".
func hasErrorsIdent(f *ast.File) bool {
	hasErrors := false
	ast.Inspect(f, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == "errors" {
			hasErrors = true
			return false
		}
		return true
	})

	return hasErrors
}
