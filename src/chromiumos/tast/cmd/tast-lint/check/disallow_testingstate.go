// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// VerifyTestingStateParam checks if functions in support packages use *testing.State as a parameter.
func VerifyTestingStateParam(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	// Ignore known valid use cases.
	var allowList = []string{
		// Runs code before and after each local test
		"src/chromiumos/tast/local/bundlemain/main.go",
		// Below files are cases still under considering.
		"src/chromiumos/tast/local/graphics/trace/trace.go",
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
			if toQualifiedName(removeStars(param.Type)) == "testing.State" {
				issues = append(issues, &Issue{
					Pos:  fs.Position(param.Type.Pos()),
					Msg:  "'testing.State' should not be used in support packages, except for precondition implementation",
					Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#test-subpackages",
				})
			}
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

// VerifyTestingStateStruct checks if testing.State is stored inside struct types.
func VerifyTestingStateStruct(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	// TODO(crbug.com/1012586): Make below file not use testing.State in struct types.
	var allowList = []string{
		"src/chromiumos/tast/local/bundles/cros/platform/memoryuser/mempressure_task.go",
	}
	filepath := fs.Position(f.Package).Filename
	for _, p := range allowList {
		if strings.HasSuffix(filepath, p) {
			return issues
		}
	}

	ast.Inspect(f, func(node ast.Node) bool {
		st, ok := node.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			if toQualifiedName(removeStars(f.Type)) == "testing.State" {
				issues = append(issues, &Issue{
					Pos:  fs.Position(f.Type.Pos()),
					Msg:  "'testing.State' should not be stored inside a struct type",
					Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#test-subpackages",
				})
			}
		}
		return true
	})
	return issues
}

// removeStars returns the exression without stars.
func removeStars(node ast.Expr) ast.Expr {
	star, ok := node.(*ast.StarExpr)
	for ok {
		node = star.X
		star, ok = node.(*ast.StarExpr)
	}
	return node
}
