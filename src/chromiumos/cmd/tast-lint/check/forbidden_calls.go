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
					Fixable: true,
				})
			} else {
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

	if tastErrorsPath := "chromiumos/tast/errors"; errorsAdded && !contains(importPathList(f), tastErrorsPath) {
		tastErrorsNode := &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: tastErrorsPath,
			},
		}
		f.Imports = append(f.Imports, tastErrorsNode)
		newf, err := ImportOrderAutoFix(fs, f)
		if err != nil {
			return issues
		}
		*f = *newf
	}

	return issues
}

func importPathList(f *ast.File) []string {
	var importlist []string
	for _, im := range f.Imports {
		importlist = append(importlist, im.Path.Value)
	}
	return importlist
}

func contains(lst []string, s string) bool {
	for _, e := range lst {
		if e == s {
			return true
		}
	}
	return false
}
