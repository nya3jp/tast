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
	// Checks for the possibility of linting fmt.Errorf with errors from "chromiumos/tast/errors" including existing
	// 'errors' identifiers (if any) and the requirement of importing errors package (if not imported).
	hasErrorsImport, errorsErr := checkErrors(f)

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
				msg := "chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf"
				if errorsErr != nil {
					msg = fmt.Sprintf("%s. Also found an error: %s", msg, errorsErr)
				}
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     msg,
					Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
					Fixable: errorsErr == nil,
				})
			} else if fixable && errorsErr == nil {
				c.Replace(&ast.SelectorExpr{
					X: &ast.Ident{
						Name: "errors",
					},
					Sel: &ast.Ident{
						Name: "Errorf",
					},
				})

				if !hasErrorsImport {
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

	// Rearrange Imports for new imports or when fmt.Errorf has been handled with conditional import.
	if len(importsRequired) > 0 || (fix && errorsErr == nil) {
		newf, err := ImportOrderAutoFix(fs, f)
		if err != nil {
			return issues
		}
		*f = *newf
	}
	return issues
}

// hasIdentErrors returns true if there is an identifier node "errors" which is not a part of
// "chromiumos/tast/errors" package, otherwise returns false.
func hasIdentErrors(f *ast.File) bool {
	hasErrors := false
	ast.Inspect(f, func(node ast.Node) bool {
		if hasErrors {
			return false // no further deep exploration
		}
		switch node.(type) {
		case *ast.SelectorExpr:
			// Removes the possibility of <exp>.<sel> with something like errors.New, errors.Wrap etc. Also it prunes the subtree from inspection.
			return false
		case *ast.Ident:
			if id := node.(*ast.Ident); id.Name == "errors" {
				hasErrors = true
				return false
			}
		}
		return true
	})

	return hasErrors
}

// checkErrors checks if the "chromiumos/tast/errors" package could replace the occurrences of fmt.Errorf
// automatically or manual intervention is solicited.
func checkErrors(f *ast.File) (bool, error) {
	if hasIdentErrors(f) {
		return false, fmt.Errorf("manual fix required, detected 'errors' as an identifier")
	}

	const pkg = "chromiumos/tast/errors"
	for _, imp := range f.Imports {
		iPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil || iPath != pkg {
			continue
		}
		// An existing import of "chromiumos/tast/errors" without alias
		if imp.Name == nil {
			return true, nil
		}

		if alias := imp.Name.Name; alias == "." || alias == "_" {
			return false, fmt.Errorf("importing '%s' as '.' or '_' is highly discouraged, please fix it", pkg)
		}
		return false, fmt.Errorf("importing '%s' with alias when there is no name conflict, is highly discouraged", pkg)
	}
	return false, nil
}
