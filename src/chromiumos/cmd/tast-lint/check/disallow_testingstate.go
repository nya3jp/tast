// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// VerifyTestingState checks if functions of support packages use *testing.State as a parameter
func VerifyTestingState(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	const docURL = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#test-subpackages"

	// Ignore known valid use cases.
	var allowList = []string{
		// Precondition files are valid use cases.
		"src/chromiumos/tast/local/arc/pre.go",
		"src/chromiumos/tast/local/chrome/pre.go",
		"src/chromiumos/tast/local/crostini/pre.go",
		// Webrtc is allowed because of the hardware reason.
		"src/chromiumos/tast/local/webrtc/camera.go",
		// Below files are cases still under considering.
		"src/chromiumos/tast/local/faillog/faillog.go",
		"src/chromiumos/tast/local/graphics/trace/trace.go",
		"src/chromiumos/tast/local/media/binsetup/binsetup.go",
	}
	filepath := fs.Position(f.Package).Filename
	for _, p := range allowList {
		if strings.HasSuffix(filepath, p) {
			return issues
		}
	}

	ast.Inspect(f, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok {
			fn, ok := node.(*ast.FuncLit)
			if !ok {
				return true
			}
			for _, param := range fn.Type.Params.List {
				n := findTestingState(param)
				if n != nil {
					issues = append(issues, &Issue{
						Pos:  fs.Position(n.Pos()),
						Msg:  "'testing.State' should not be used in support packages",
						Link: docURL,
					})
				}
			}
			return true
		}
		for _, param := range fn.Type.Params.List {
			n := findTestingState(param)
			if n != nil {
				issues = append(issues, &Issue{
					Pos:  fs.Position(n.Pos()),
					Msg:  "'testing.State' should not be used in support packages",
					Link: docURL,
				})
			}
		}
		return true
	})

	return issues
}

func findTestingState(param *ast.Field) *ast.SelectorExpr {
	st, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return nil
	}
	n, ok := st.X.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	if n.Sel.Name == "State" {
		id, ok := n.X.(*ast.Ident)
		if !ok {
			return nil
		}
		if id.Name == "testing" {
			return n
		}
	}
	return nil
}
