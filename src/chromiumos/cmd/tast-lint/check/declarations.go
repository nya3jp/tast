// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strconv"
	"unicode"
)

// Declarations checks declarations of testing.Test structs.
func Declarations(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isTestMainFile(filename) {
		return nil
	}

	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		comp, ok := node.(*ast.CompositeLit)
		if !ok {
			return
		}
		if sel, ok := comp.Type.(*ast.SelectorExpr); !ok {
			return
		} else if x, ok := sel.X.(*ast.Ident); !ok {
			return
		} else if x.Name != "testing" || sel.Sel.Name != "Test" {
			return
		}

		for _, el := range comp.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			var key string
			if ident, ok := kv.Key.(*ast.Ident); !ok {
				continue
			} else {
				key = ident.Name
			}
			switch key {
			case "Desc":
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						if issue := checkTestDesc(s); issue != nil {
							issue.Pos = fs.Position(lit.Pos())
							issues = append(issues, issue)
						}
					}
				}
			}
		}
	})

	ast.Walk(v, f)
	return issues
}

// Exposed here for unit tests.
const badDescMsg = `Test descriptions should be capitalized phrases without trailing punctuation, e.g. "Checks that foo is bar"`

// checkTestDesc inspects a testing.Test.Desc string.
// If problems are found, a non-nil value is returned.
// The caller is responsible for filling the Pos field in the returned Issue.
func checkTestDesc(desc string) *Issue {
	if desc == "" || !unicode.IsUpper(rune(desc[0])) || desc[len(desc)-1] == '.' {
		return &Issue{
			Msg:  badDescMsg,
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-names",
		}
	}
	return nil
}
