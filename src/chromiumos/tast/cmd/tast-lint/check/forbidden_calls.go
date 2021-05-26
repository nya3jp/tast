// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"unicode"

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
	// checks for "chromiumos/tast/errors" import, if the file already has it, no reimport
	chkImport, chkError := requireImportErrors(f)

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
				if chkError != nil {
					msg = fmt.Sprintf("%s. Also found error: %s", msg, chkError)
				}
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     msg,
					Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
					Fixable: chkError == nil,
				})
			} else if fixable && chkError == nil {
				c.Replace(&ast.SelectorExpr{
					X: &ast.Ident{
						Name: "errors",
					},
					Sel: &ast.Ident{
						Name: "Errorf",
					},
				})

				if !chkImport {
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

	// Rearrange Imports for new Imports or fmt.Errorf has been handled with conditional import.
	if len(importsRequired) > 0 || (fix && chkError == nil) {
		newf, err := ImportOrderAutoFix(fs, f)
		if err != nil {
			return issues
		}
		*f = *newf
	}
	return issues
}

// hasIdentErrors returns true if there is an identifier node "errors" which is not a part of
// "chromiumos/tast/errors" package otherwise false.
func hasIdentErrors(f *ast.File) bool {
	// if errors.[UPPERCASE]<Rest> -> from errors package
	hasErrors, analyzenxt := false, false
	ast.Inspect(f, func(node ast.Node) bool {
		if hasErrors && !analyzenxt {
			return false // overwrite prevention
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if analyzenxt {
			analyzenxt = false
			if n := id.Name; len(n) > 0 && !unicode.IsUpper([]rune(n)[0]) {
				return false // found independent errors node
			}
			hasErrors = false
		}
		if id.Name == "errors" {
			hasErrors = true
			analyzenxt = true
		}
		return true
	})

	return hasErrors
}

// requireImportErrors checks if "chromiumos/tast/errors" package could imported or not.
func requireImportErrors(f *ast.File) (bool, error) {
	pkg := "chromiumos/tast/errors"
	// checks for an existing independent identifier 'errors', if found manual fix is required
	if hasIdentErrors(f) {
		return false, fmt.Errorf("manual fix required, detected '%s' as an identifier", pkg)
	}

	for _, imp := range f.Imports { // iterate over existing imports
		iPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil || iPath != pkg {
			continue
		}
		// an existing import of "chromiumos/tast/errors" without alias
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
