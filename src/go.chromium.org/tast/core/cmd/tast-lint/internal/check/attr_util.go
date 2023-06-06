// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// testAttrChecker is a function checks for a test's Attr and ExtraAttr.
type testAttrChecker func(attrs []string, pos token.Position) []*Issue

func checkAttr(fs *token.FileSet, f *ast.File, checker testAttrChecker) []*Issue {
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
					issues = append(issues, checker(exAttrs, fs.Position(exAttrPos))...)
				}
			} else {
				issues = append(issues, checker(attrs, fs.Position(attrPos))...)
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
