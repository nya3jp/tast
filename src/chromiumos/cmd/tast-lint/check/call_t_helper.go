// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// VerifyCallingTHelper checks if unit test helper functions calling testing.T.Helper inside them.
func VerifyCallingTHelper(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	for _, decl := range f.Decls {
		if !isHelperTestFunc(decl) || !hasTestingT(decl) || !callTFatalError(decl) {
			continue
		}
		fn := decl.(*ast.FuncDecl)
		callTHelper := false
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if toQualifiedName(call.Fun) == "t.Helper" {
				callTHelper = true
				return false
			}
			return true
		})
		if !callTHelper && timesCalledByTestFuncs(f, fn.Name.Name) >= 2 {
			issues = append(issues, &Issue{
				Pos:  fs.Position(fn.Name.Pos()),
				Msg:  fmt.Sprintf("testing.T.Helper should be called inside the helper function %s()", fn.Name.Name),
				Link: "https://golang.org/pkg/testing/#T.Helper",
			})
		}
	}
	return issues
}

// callFunction returns the list of the name of functions which call a function that has given name.
func callFunction(f *ast.File, funcName string) []string {
	var callFunc []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if toQualifiedName(fn.Name) == funcName {
			continue // to avoid catching a recursive function
		}

		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if toQualifiedName(call.Fun) == funcName { // call.Fun is Expr
				callFunc = append(callFunc, toQualifiedName(fn.Name))
				return false
			}
			return true
		})
	}
	return callFunc
}

// makeUniqueStrSlice removes duplicate value from given slice and returns it.
func makeUniqueStrSlice(dupSlice []string) []string {
	m := make(map[string]bool)
	var uniqSlice []string
	for _, e := range dupSlice {
		if !m[e] {
			m[e] = true
			uniqSlice = append(uniqSlice, e)
		}
	}
	return uniqSlice
}

// timesCalledByTestFuncs returns how many kinds of functions call a function that has given name.
func timesCalledByTestFuncs(f *ast.File, funcName string) int {
	callFuncList := makeUniqueStrSlice(callFunction(f, funcName))
	for i, fn := range callFuncList {
		if strings.HasPrefix(fn, "Test") {
			continue
		}
		callFuncList = removeElementFromSlice(i, callFuncList)
		callFuncList = append(callFuncList, callFunction(f, fn)...)
		callFuncList = makeUniqueStrSlice(callFuncList)
	}
	return len(callFuncList)
}

// removeElementFromSlice removes the element at index i from given slice and return it.
func removeElementFromSlice(i int, sl []string) []string {
	copy(sl[i:], sl[i+1:])
	sl[len(sl)-1] = ""
	sl = sl[:len(sl)-1]
	return sl
}

func isHelperTestFunc(node ast.Node) bool {
	fn, ok := node.(*ast.FuncDecl)
	if !ok {
		return false
	}
	if strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	return true
}

// hasTestingT returns true if the function of given node has "t *testing.T" as a parameter.
func hasTestingT(node ast.Node) bool {
	fn, ok := node.(*ast.FuncDecl)
	if !ok {
		return false
	}
	foundTestingT := false
	foundVarT := false
	for _, param := range fn.Type.Params.List {
		st, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if _, ok := st.X.(*ast.SelectorExpr); !ok {
			continue
		}
		if toQualifiedName(st.X) == "testing.T" {
			foundTestingT = true
			for _, id := range param.Names {
				if id.Name == "t" {
					foundVarT = true
				}
			}
		}
	}
	return foundTestingT && foundVarT
}

// callTFatalError returns true if the function of given node calls t.Fatal/t.Fatalf/t.Error/t.Errorf.
func callTFatalError(node ast.Node) bool {
	foundTFatalError := false
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		nm := toQualifiedName(call.Fun)
		if nm == "t.Fatal" || nm == "t.Fatalf" || nm == "t.Error" || nm == "t.Errorf" {
			foundTFatalError = true
			return false
		}
		return true
	})
	return foundTFatalError
}
