// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

// ForbiddenCalls checks if any forbidden functions are called.
func ForbiddenCalls(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	isUnitTest := isUnitTestFile(fs.Position(f.Package).Filename)
	var issues []*Issue
	var importsRequired []string

	// Being able to run goimports is a precondition to being able to make any fixes.
	fixable := false
	if src, err := formatASTNode(fs, f); err == nil {
		fixable = goimportApplicable(src)
	}
	// If a file contains "chromium/tast/errors" import, it will check for available alias or
	// reimport the package without a namespace conflict.
	errorsAlias, usesErrors := getErrorsImportAlias(f)

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

		methodName := sel.Sel.Name
		call := fmt.Sprintf("%s.%s", x.Name, methodName)
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
						Name: errorsAlias,
					},
					Sel: &ast.Ident{
						Name: "Errorf",
					},
				})
				// If it is already imported, re-import is not required.
				if !usesErrors {
					importsRequired = append(importsRequired, "chromiumos/tast/errors")
				}
			}
		case "time.Sleep":
			issues = append(issues, &Issue{
				Pos:  fs.Position(x.Pos()),
				Msg:  "time.Sleep ignores context deadline; use testing.Poll or testing.Sleep instead",
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
			})
		case "dbus.SystemBus", "dbus.SystemBusPrivate":
			if !fix {
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     fmt.Sprintf("dbus.%v may reorder signals; use chromiumos/tast/local/dbusutil.%v instead", methodName, methodName),
					Link:    "https://github.com/godbus/dbus/issues/187",
					Fixable: fixable,
				})
			} else if fixable {
				c.Replace(&ast.SelectorExpr{
					X: &ast.Ident{
						Name: "dbusutil",
					},
					Sel: &ast.Ident{
						Name: methodName,
					},
				})
				importsRequired = append(importsRequired, "chromiumos/tast/local/dbusutil")
			}
		}

		return true
	}, nil)

	for _, pkg := range importsRequired {
		astutil.AddImport(fs, f, pkg)
	}
	if len(importsRequired) > 0 {
		newf, err := ImportOrderAutoFix(fs, f)
		if err != nil {
			return issues
		}
		*f = *newf
	}
	return issues
}

// getErrorsImportAlias checks if errors package is imported and returns suitable import alias.
func getErrorsImportAlias(f *ast.File) (string, bool) {
	var isImported bool
	importAlias := "errors"

	for _, imp := range f.Imports {
		iPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if iPath == "chromiumos/tast/errors" {
			if imp.Name != nil {
				// Capture import alias
				alias := imp.Name.Name
				if alias == "." || alias == "_" {
					// To avoid namespace collision (for Dot import) it's better to re-import errors package.
					continue
				} else {
					importAlias = alias
				}
			}
			isImported = true
			// If no alias is present for errors import, we'll use it. No further iteration is required.
			break
		}
	}
	return importAlias, isImported
}
