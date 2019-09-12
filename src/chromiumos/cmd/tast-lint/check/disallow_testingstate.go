// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// TestingStateCheck checks if functions of support packages use *testing.State as a parameter
func TestingStateCheck(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	const docURL = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#test-subpackages"
	// ignore valid use cases
	filepath := fs.Position(f.Package).Filename
	if isOfWhiteList(filepath) {
		return issues
	}

	for _, decl := range f.Decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok {
				return false
			}

			for _, param := range fn.Type.Params.List {
				var comp []string
				st, ok2 := param.Type.(*ast.StarExpr)
				if ok2 {
					n, ok3 := st.X.(*ast.SelectorExpr)
					if ok3 {
						comp = append(comp, n.Sel.Name)
						id, ok4 := n.X.(*ast.Ident)
						if ok4 {
							comp = append(comp, id.Name)
						}
						for i, j := 0, len(comp)-1; i < j; i, j = i+1, j-1 {
							comp[i], comp[j] = comp[j], comp[i]
						}

						typeName := strings.Join(comp, ".")

						if typeName == "testing.State" {
							issues = append(issues, &Issue{
								Pos:  fs.Position(n.Pos()),
								Msg:  "'testing.State' should not be used in support packages",
								Link: docURL,
							})
						}

					}
				}
			}

			return false
		})
	}

	return issues
}

func isOfWhiteList(filepath string) bool {
	// add if needed
	whitelist := []string{
		"../tast-tests/src/chromiumos/tast/local/arc/pre.go",
		"../tast-tests/src/chromiumos/tast/local/chrome/pre.go",
		"../tast-tests/src/chromiumos/tast/local/crostini/pre.go",
		"../tast-tests/src/chromiumos/tast/local/webrtc/camera.go",
	}

	for _, p := range whitelist {
		if p == filepath {
			return true
		}
	}
	return false
}
