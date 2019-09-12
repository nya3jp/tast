// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// VerifyTestingState checks if functions in support packages use *testing.State as a parameter.
func VerifyTestingState(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	// Ignore known valid use cases.
	var allowList = []string{
		// Precondition files are valid use cases.
		"src/chromiumos/tast/local/arc/pre.go",
		"src/chromiumos/tast/local/chrome/pre.go",
		"src/chromiumos/tast/local/crostini/pre.go",
		// Below files are cases still under considering.
		"src/chromiumos/tast/local/webrtc/camera.go",
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
		t := funcType(node)
		if t == nil {
			return true
		}
		for _, param := range t.Params.List {
			st, ok := param.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			n, ok := st.X.(*ast.SelectorExpr)
			if !ok || n.Sel.Name != "State" {
				continue
			}
			if id, ok := n.X.(*ast.Ident); !ok || id.Name != "testing" {
				continue
			}
			issues = append(issues, &Issue{
				Pos:  fs.Position(n.Pos()),
				Msg:  "'testing.State' should not be used in support packages, except for precondition implementation",
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#test-subpackages",
			})
		}
		return true
	})

	return issues
}

func funcType(node ast.Node) *ast.FuncType {
	if fn, ok := node.(*ast.FuncDecl); ok {
		return fn.Type
	}
	if fn, ok := node.(*ast.FuncLit); ok {
		return fn.Type
	}
	return nil
}
