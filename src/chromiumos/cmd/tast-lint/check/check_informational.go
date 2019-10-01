// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

const (
	shouldBeInformationalMsg = `Newly added tests should be marked as 'informational'.`
)

// VerifyInformationalAttr checks if a newly added test has 'informational' attribute.
func VerifyInformationalAttr(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "init" {
			// Not an init() function declaration. Skip.
			continue
		}
		for _, stmt := range fn.Body.List {
			estmt, ok := stmt.(*ast.ExprStmt)
			if !ok || !isTestingAddTestCall(estmt.X) {
				continue
			}
			call := estmt.X.(*ast.CallExpr)
			if len(call.Args) != 1 {
				// This should be checked by a compiler, so skipped.
				continue
			}
			// Verify the argument is "&testing.Test{...}"
			arg, ok := call.Args[0].(*ast.UnaryExpr)
			if !ok || arg.Op != token.AND {
				continue
			}
			comp, ok := arg.X.(*ast.CompositeLit)
			if !ok {
				continue
			}

			var identName string
			var kvValue ast.Node
			var attrPos token.Pos
			var attrs []string
			isParameterizedTest := false
			var exComp *ast.CompositeLit
			var exOk bool

			for _, el := range comp.Elts {
				identName, kvValue, attrPos = nameIdentValPos(el)
				if identName == "Attr" {
					attrs = makeSliceAttrs(kvValue)
				}
				if identName == "Params" {
					isParameterizedTest = true
					exComp, exOk = kvValue.(*ast.CompositeLit)
				}
			}

			if !isParameterizedTest && isMainlineNoInfo(attrs) {
				issues = append(issues, &Issue{
					Pos:  fs.Position(attrPos),
					Msg:  shouldBeInformationalMsg,
					Link: testRegistrationURL,
				})
			} else if isParameterizedTest {
				if !exOk {
					continue
				}
				for _, el := range exComp.Elts {
					comp, ok := el.(*ast.CompositeLit)
					if !ok {
						continue
					}
					var exAttrPos token.Pos
					var exAttrs []string
					for _, el := range comp.Elts {
						identName, kvValue, exAttrPos = nameIdentValPos(el)
						if identName == "ExtraAttr" {
							exAttrs = makeSliceAttrs(kvValue)
						}
					}
					exAttrs = append(exAttrs, attrs...)
					if !isMainlineNoInfo(exAttrs) {
						continue
					}
					issues = append(issues, &Issue{
						Pos:  fs.Position(exAttrPos),
						Msg:  shouldBeInformationalMsg,
						Link: testRegistrationURL,
					})
				}
			}
		}
	}
	return issues
}

// nameIdentValPos returns the name of identifier, node of keyvalue and its position.
func nameIdentValPos(el ast.Node) (string, ast.Node, token.Pos) {
	kv, ok := el.(*ast.KeyValueExpr)
	if !ok {
		return "", nil, token.NoPos
	}
	ident, ok := kv.Key.(*ast.Ident)
	if !ok {
		return "", kv.Value, kv.Pos()
	}
	return ident.Name, kv.Value, kv.Pos()
}

// makeSliceAttrs make a string slice of elements in attribute.
// We do not check the non-string-literal elements.
func makeSliceAttrs(node ast.Node) []string {
	var attrs []string
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return attrs
	}
	for _, el := range comp.Elts {
		s, ok := toString(el)
		if !ok {
			continue
		}
		attrs = append(attrs, s)
	}
	return attrs
}

// isMainlineNoInfo returns true if there are no 'informational' attribute in existing attributes
// and also it is mainline test.
func isMainlineNoInfo(attrs []string) bool {
	isMainlineTest := true
	hasInformational := false
	for _, attr := range attrs {
		if attr == "informational" {
			hasInformational = true
		} else if attr != "group:mainline" && strings.HasPrefix(attr, "group:") {
			isMainlineTest = false
		}
	}
	return isMainlineTest && !hasInformational
}
