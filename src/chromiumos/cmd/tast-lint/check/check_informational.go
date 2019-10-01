// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
)

const (
	shouldBeInformationalMsg = `Newly added tests should be marked as 'informational'.`
)

// VerifyInformationalAttr checks if a newly added test has 'informational' attribute.
func VerifyInformationalAttr(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue
	var paramIssues []*Issue

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

			isParameterizedTest := false
			var attrs []string
			var identName string
			var kvValue ast.Node
			var attrPos token.Pos

			for _, el := range comp.Elts {
				identName, kvValue, attrPos = verifyIdent(el)

				if identName == "Attr" {
					attrs = makeSliceAttrs(kvValue)
				}

				if identName != "Params" {
					continue
				}
				isParameterizedTest = true
				comp, ok := kvValue.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, el := range comp.Elts {
					var exAttrs []string
					var exAttrPos token.Pos
					comp, ok := el.(*ast.CompositeLit)
					if !ok {
						continue
					}
					for _, el := range comp.Elts {
						identName, kvValue, exAttrPos = verifyIdent(el)

						if identName == "ExtraAttr" {
							exAttrs = makeSliceAttrs(kvValue)
							break
						}
					}
					if !shouldBeWarned(exAttrs) {
						continue
					}
					paramIssues = append(paramIssues, &Issue{
						Pos:  fs.Position(exAttrPos),
						Msg:  shouldBeInformationalMsg,
						Link: testRegistrationURL,
					})
				}
			}

			if !shouldBeWarned(attrs) {
				continue
			}
			if isParameterizedTest {
				issues = append(issues, paramIssues...)
			} else {
				issues = append(issues, &Issue{
					Pos:  fs.Position(attrPos),
					Msg:  shouldBeInformationalMsg,
					Link: testRegistrationURL,
				})
			}
		}
	}
	return issues
}

// verifyIdent returns the name of identifier, node of keyvalue and its position.
func verifyIdent(el ast.Node) (string, ast.Node, token.Pos) {
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

// shouldBeWarned returns true if there are no 'informational' attribute in existing attributes
// and also it is mainline test.
func shouldBeWarned(attrs []string) bool {
	isMainlineTest := true
	hasInformational := false
	for _, attr := range attrs {
		if attr == "informational" {
			hasInformational = true
		} else if attr != "group:mainline" {
			isMainlineTest = false
		}
	}
	return isMainlineTest && !hasInformational
}
