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
	// reqImport preserves the imports info with path and alias name
	// eg. import name "path"
	type reqImport struct {
		path string
		name string
	}
	var importsRequired []reqImport

	// Being able to run goimports is a precondition to being able to make any fixes.
	fixable := false
	if src, err := formatASTNode(fs, f); err == nil {
		fixable = goimportApplicable(src)
	}
	// If a file contains "chromiumos/tast/errors" import, it will check for available alias or it will generate
	// suitable alias based on the current environment.
	errAlias, errImported, errError := errorsImportAlias(f)

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
			} else if errError != nil {
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     fmt.Sprintf("chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf. %s", errError),
					Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
					Fixable: false,
				})
			} else if fixable {
				c.Replace(&ast.SelectorExpr{
					X: &ast.Ident{
						Name: errAlias,
					},
					Sel: &ast.Ident{
						Name: "Errorf",
					},
				})
				// if it is not imported, import with suitable alias
				if !errImported {
					importAlias := errAlias
					if importAlias == "errors" {
						importAlias = ""
					}
					importsRequired = append(importsRequired, reqImport{
						path: "chromiumos/tast/errors",
						name: importAlias,
					})
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
				importsRequired = append(importsRequired, reqImport{path: "chromiumos/tast/local/dbusutil"})
			}
		}

		return true
	}, nil)

	for _, pkg := range importsRequired {
		if pkg.name == "" {
			astutil.AddImport(fs, f, pkg.path)
		} else {
			astutil.AddNamedImport(fs, f, pkg.name, pkg.path)
		}
	}

	// Rearrange Imports for new Imports or fmt.Errorf has been handled with conditional import.
	if len(importsRequired) > 0 || (fix && errError == nil) {
		newf, err := ImportOrderAutoFix(fs, f)
		if err != nil {
			return issues
		}
		*f = *newf
	}
	return issues
}

// hasIdent returns true if there is an identifier node.
func hasIdent(f *ast.File, identifier string) bool {
	hasErrors := false
	ast.Inspect(f, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == identifier {
			hasErrors = true
			return false
		}
		return true
	})

	return hasErrors
}

// errorsImportAlias checks if errors package is imported and returns suitable import alias.
func errorsImportAlias(f *ast.File) (string, bool, error) {
	var isImported bool
	importAlias := ""

	for _, imp := range f.Imports {
		iPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil || iPath != "chromiumos/tast/errors" {
			continue
		}

		// capture import alias
		if imp.Name != nil {
			alias := imp.Name.Name
			// tast-lint won't handle errors if the "chromiumos/tast/errors" has been imported with dot or underscore import.
			if alias == "." || alias == "_" {
				return "", false, fmt.Errorf("tast-lint can't fix fmt.Errorf as errors has used as dot(\".\")" +
					"underscore(\"_\") import")
			}
			importAlias = alias
		}
		isImported = true
		break
	}
	// already imported with an alias. So tast-lint thinks the alias has been used in the program at least once.
	// Else the go compiler will throw an error for non used imports.
	if importAlias != "" {
		return importAlias, isImported, nil
	}
	importAlias = "errors"
	hasErrorsNode := hasIdent(f, importAlias)

	if hasErrorsNode {
		// already imported with no alias and there is a function or variable name errors. tast-lint won't resolve this.
		if isImported {
			return "", false, fmt.Errorf("manual fix is required. Name conflict detected, " +
				"\"chromiumos/tast/errors\" with an unintended instance of 'errors' as an identifier")
		}

		// if "chromiumos/tast/errors" hasn't yet been imported tast-lint will try to come up with a unique alias.
		for i := 1; i < 10; i++ {
			importAlias = fmt.Sprintf("errors%d", i)
			if !hasIdent(f, importAlias) {
				// suitable alias found
				return importAlias, isImported, nil
			}
		}
		return "", false, fmt.Errorf("tast-lint tried to solve name conflict by using different import aliases" +
			"but the program consists identifiers from errors1...10. Manual fix required")
	}

	return importAlias, isImported, nil
}
