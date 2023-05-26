// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// hasImport checks if the import declaration contains specified package name.
// If so, it returns the name of the import, and an empty string otherwise.
func hasImport(f *ast.File, pkgName string) string {
	sfmt := fmt.Sprintf("\"%s\"", pkgName)
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}

		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok || importSpec.Path.Kind != token.STRING {
				continue
			}

			if importSpec.Path.Value == sfmt {
				if importSpec.Name != nil {
					return importSpec.Name.Name
				}

				sparts := strings.Split(pkgName, "/")
				return sparts[len(sparts)-1]
			}
		}

		break
	}

	return ""
}

// hasTastTestInit checks if the given function declaration is an init function
// which declares a Tast test. If so, it returns true, and false otherwise.
func hasTastTestInit(f ast.FuncDecl, testingPkgName string) bool {
	if f.Name.Name != "init" || len(f.Body.List) != 1 {
		return false
	}

	exprStmt, ok := f.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}

	callExpr, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}

	selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok || selectorExpr.Sel.Name != "AddTest" {
		return false
	}

	ident, ok := selectorExpr.X.(*ast.Ident)
	if !ok || ident.Name != testingPkgName {
		return false
	}

	return true
}

// extractFunctions extracts all top-level function declarations from the given
// File. It returns a map where the key is the name of the function, and the
// value is the function declaration.
func extractFunctions(f *ast.File) map[string]ast.FuncDecl {
	m := make(map[string]ast.FuncDecl)
	for _, decl := range f.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		m[funcDecl.Name.Name] = *funcDecl
	}

	return m
}

// extractValue tries to extract the value from a SelectorExpr, or a string
// BasicLit. If successful, it returns the string value and the position of the
// token. Otherwise, it returns an empty string and token.NoPos.
// When the allowedValues is not nil, it returns only the values that are keys
// in the map, and that have a value of true.
func extractValue(node ast.Node, allowedValues map[string]bool, pkgName string) (string, token.Pos) {
	var value string

	// Check if the value is defined as a selector expression.
	selectorExpr, ok := node.(*ast.SelectorExpr)
	if ok {
		ident, ok := selectorExpr.X.(*ast.Ident)
		if ok && ident.Name == pkgName {
			value = selectorExpr.Sel.Name
		}
	}

	// Check if the value is defined as a string literal.
	basicLit, ok := node.(*ast.BasicLit)
	if ok && basicLit.Kind == token.STRING && len(basicLit.Value) > 2 {
		value = basicLit.Value[1 : len(basicLit.Value)-1]
	}

	if allowedValues != nil {
		val, ok := allowedValues[value]
		if !ok || !val {
			return "", token.NoPos
		}
	}

	return value, node.Pos()
}

// extractValues tries to extract the value from the given nodes.
// To decide which values to select, extractValue is used.
func extractValues(nodes []ast.Node, allowedValues map[string]bool, pkgName string) map[string]token.Pos {
	m := make(map[string]token.Pos)

	// Find all Values in the Search Flags.
	fv := funcVisitor(func(node ast.Node) {
		value, pos := extractValue(node, allowedValues, pkgName)
		if pos == token.NoPos {
			return
		}

		m[value] = pos
	})

	for _, node := range nodes {
		ast.Walk(fv, node)
	}

	return m
}

// extractSearchFlagNodes tries to extract the value from the SearchFlags and
// ExtraSearchFlags Key-Value pairs. It returns a slice with the given nodes.
func extractSearchFlagNodes(f *ast.File) []ast.Node {
	var nodes []ast.Node

	// Find all SearchFlag and ExtraSearchFlag declarations.
	fv := funcVisitor(func(node ast.Node) {
		kvExpr, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return
		}

		keyIdent, ok := kvExpr.Key.(*ast.Ident)
		if !ok || (keyIdent.Name != "SearchFlags" && keyIdent.Name != "ExtraSearchFlags") {
			return
		}

		nodes = append(nodes, kvExpr.Value)
	})

	ast.Walk(fv, f)

	return nodes
}

// extractSearchFlagValues tries to extract the value from the SearchFlags and
// ExtraSearchFlags Key-Value pairs. It returns a slice with the given nodes.
func extractSearchFlagValues(nodes []ast.Node, funcs map[string]ast.FuncDecl, allowedValues map[string]bool, pkgName string) (map[string]token.Pos, bool) {
	m := make(map[string]token.Pos)

	var nonFunctionNodes []ast.Node
	for _, node := range nodes {
		callExpr, ok := node.(*ast.CallExpr)
		if !ok {
			nonFunctionNodes = append(nonFunctionNodes, node)
			continue
		}

		var name string
		ident, ok := callExpr.Fun.(*ast.Ident)
		if ok {
			name = ident.Name
		} else {
			selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			ident, ok = selectorExpr.X.(*ast.Ident)
			if !ok {
				continue
			}

			name = fmt.Sprintf("%s.%s", ident.Name, selectorExpr.Sel.Name)
		}

		funcDecl, ok := funcs[name]
		if !ok {
			// We have an imported function.
			return nil, true
		}

		// We have a function in the same file.
		vals := extractValues([]ast.Node{funcDecl.Body}, allowedValues, pkgName)
		m = union(m, vals)
	}

	vals := extractValues(nonFunctionNodes, allowedValues, pkgName)
	m = union(m, vals)

	return m, false
}

// extractTestFileValues tries to extract the value from the file based on the
// extractValue function. If ignoredValues is not nil, the values that have the
// specified position will be ignored.
func extractTestFileValues(f *ast.File, values map[string]bool, pkgName string, ignoredValues map[string]token.Pos) map[string]token.Pos {
	m := make(map[string]token.Pos)

	// Find all Values in the file.
	fv := funcVisitor(func(node ast.Node) {
		value, pos := extractValue(node, values, pkgName)
		if pos == token.NoPos {
			return
		}

		if ignoredValues != nil {
			ipos, ok := ignoredValues[value]
			if ok && ipos == pos {
				return
			}
		}

		m[value] = pos
	})

	ast.Walk(fv, f)

	return m
}

// SearchFlags checks search flags in Tast tests definitions for policy names
// that have been used during the testing phase. If the file is not a Tast test,
// then it is skipped.
func SearchFlags(fs *token.FileSet, f *ast.File) (issues []*Issue) {
	policyPkgName := hasImport(f, "go.chromium.org/tast-tests/cros/common/policy")
	if policyPkgName == "" {
		return
	}

	testingPkgName := hasImport(f, "go.chromium.org/tast/core/testing")
	if testingPkgName == "" {
		return
	}

	functions := extractFunctions(f)
	if init, ok := functions["init"]; !ok || !hasTastTestInit(init, testingPkgName) {
		return
	}

	policyNames := policyNames()
	nodes := extractSearchFlagNodes(f)
	tags, ok := extractSearchFlagValues(nodes, functions, policyNames, policyPkgName)
	if ok {
		return
	}

	usedPolicies := extractTestFileValues(f, policyNames, policyPkgName, tags)

	for k, v := range usedPolicies {
		_, ok := tags[k]
		if ok {
			continue
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(v),
			Msg:  fmt.Sprintf("Policy %s does not have a corresponding Search Flag.", k),
			Link: "go/remote-management/tast-codelabs/policy_coverage_insights",
		})
	}

	return
}
