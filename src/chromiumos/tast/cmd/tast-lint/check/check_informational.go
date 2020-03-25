// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
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

			var attrs []string
			var attrPos token.Pos
			var paramNode ast.Node
			for _, el := range comp.Elts {
				identName, value, pos, err := decomposeKVNode(el)
				if err != nil {
					continue
				}
				if identName == "Attr" {
					attrs = makeSliceAttrs(value)
					attrPos = pos
				}
				if identName == "Params" {
					paramNode = value
				}
			}

			if paramNode != nil {
				comp, ok := paramNode.(*ast.CompositeLit)
				if !ok {
					// This should be checked by another lint, so skipped.
					continue
				}
				for _, el := range comp.Elts {
					comp, ok := el.(*ast.CompositeLit)
					if !ok {
						continue
					}
					var exAttrs []string
					var exAttrPos token.Pos
					for _, el := range comp.Elts {
						identName, value, pos, err := decomposeKVNode(el)
						if err != nil {
							continue
						}
						if identName == "ExtraAttr" {
							exAttrs = makeSliceAttrs(value)
							exAttrPos = pos
						}
					}
					exAttrs = append(exAttrs, attrs...)
					if isCriticalTest(exAttrs) {
						issues = append(issues, &Issue{
							Pos:  fs.Position(exAttrPos),
							Msg:  shouldBeInformationalMsg,
							Link: testRegistrationURL,
						})
					}
				}
			} else {
				if isCriticalTest(attrs) {
					issues = append(issues, &Issue{
						Pos:  fs.Position(attrPos),
						Msg:  shouldBeInformationalMsg,
						Link: testRegistrationURL,
					})
				}
			}
		}
	}
	return issues
}

// decomposeKVNode returns the name of identifier, node of keyvalue and its position.
func decomposeKVNode(el ast.Node) (string, ast.Node, token.Pos, error) {
	kv, ok := el.(*ast.KeyValueExpr)
	if !ok {
		return "", nil, token.NoPos, fmt.Errorf("unexpected input")
	}
	ident, ok := kv.Key.(*ast.Ident)
	if !ok {
		return "", kv.Value, kv.Pos(), fmt.Errorf("unexpected input")
	}
	return ident.Name, kv.Value, kv.Pos(), nil
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

// isCriticalTest returns true if there are no 'informational' attribute
// in existing attributes and it is mainline test.
func isCriticalTest(attrs []string) bool {
	isMainlineTest := false
	isInformational := false
	for _, attr := range attrs {
		if attr == "informational" {
			isInformational = true
		} else if attr == "group:mainline" {
			isMainlineTest = true
		}
	}
	return isMainlineTest && !isInformational
}
