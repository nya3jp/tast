// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

const (
	noLacrosStatusMsg           = `Test LacrosStatus field should exist`
	nonLiteralLacrosMetadataMsg = `Test LacrosStatus should be a single LacrosStatus value, e.g. testing.LacrosVariantNeeded`
	noLacrosVariantUnknownMsg   = `Test LacrosStatus should not be LacrosVariantUnknown. Please work out if your test needs lacros variants. To do this, see go/lacros-tast-porting or contact edcourtney@, hidehiko@, or lacros-tast@`
	addedUnknownMetadataMsg     = `tast-lint has added LacrosVariantUnknown as a placeholder but please work out if your test needs lacros variants. To do this, see go/lacros-tast-porting or contact edcourtney@, hidehiko@, or lacros-tast@`
	noLacrosSoftwareDepsMsg     = `Test SoftwareDeps dep:lacros should be added along with dep:lacros_{un}stable`
)

func hasSoftwareDeps(expr ast.Expr, dep string) bool {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return v.Value == "\""+dep+"\""
	case *ast.Ident:
	case *ast.SelectorExpr:
		// We can't really compute the value of constants so just assume it
		// was the dep value we are looking for.
		return true
	case *ast.CompositeLit:
		for _, arg := range v.Elts {
			if hasSoftwareDeps(arg, dep) {
				return true
			}
		}
		return false
	case *ast.CallExpr:
		fun, ok := v.Fun.(*ast.Ident)
		if !ok || fun.Name != "append" {
			return false
		}
		for i, arg := range v.Args {
			isVarList := i == 0 || (i == len(v.Args)-1 && v.Ellipsis != token.NoPos)
			if isVarList {
				if hasSoftwareDeps(arg, dep) {
					return true
				}
			}
			if !isVarList && !hasSoftwareDeps(arg, dep) {
				return false
			}
		}
		return false
	}

	// If we can't show there is no software dep, assume it exists.
	return true
}

func checkAllSoftwareDeps(fields entityFields, dep string) bool {
	// Check for chrome SoftwareDeps.
	s, ok := fields["SoftwareDeps"]
	if ok && hasSoftwareDeps(s.Value, dep) {
		return true
	}

	// Check ExtraSoftwareDeps:
	s, ok = fields["Params"]
	if !ok {
		return false // Was not in SoftwareDeps and no Params.
	}

	v, ok := s.Value.(*ast.CompositeLit)
	if !ok {
		return true // Expect Params to be a CompositeLit.
	}

	for _, arg := range v.Elts {
		// Extract each Param
		v, ok := arg.(*ast.CompositeLit)
		if !ok {
			return false
		}

		for _, paramField := range v.Elts {
			kv, ok := paramField.(*ast.KeyValueExpr)
			if !ok {
				return false
			}
			id, ok := kv.Key.(*ast.Ident)
			if !ok {
				return false
			}

			if id.Name == "ExtraSoftwareDeps" && hasSoftwareDeps(kv.Value, dep) {
				return true
			}
		}
	}

	return false
}

func maybeRewrite(fs *token.FileSet, fields entityFields, call *ast.CallExpr, fix bool, issues []*Issue) []*Issue {
	if fix && len(issues) > 0 {
		f := &ast.KeyValueExpr{
			Key: &ast.Ident{
				Name: "LacrosStatus",
			},
			Value: &ast.SelectorExpr{
				X: &ast.Ident{
					Name: "testing",
				},
				Sel: &ast.Ident{
					Name: "LacrosVariantUnknown",
				},
			},
		}
		if kv, ok := fields["LacrosStatus"]; ok {
			// Try rewriting the field if it already exists.
			astutil.Apply(kv, func(c *astutil.Cursor) bool {
				if p, ok := c.Parent().(*ast.KeyValueExpr); ok && c.Node() == p.Value {
					c.Replace(f.Value)
				}
				return true
			}, nil)
		} else {
			// Otherwise add it after Func which should exist.
			// TODO: This won't add newlines, and there doesn't appear to be any
			// way to do this using astutil.
			astutil.Apply(call, func(c *astutil.Cursor) bool {
				_, parentIsComposite := c.Parent().(*ast.CompositeLit)
				kv, currentIsKeyValue := c.Node().(*ast.KeyValueExpr)
				if parentIsComposite && currentIsKeyValue {
					if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Func" {
						c.InsertAfter(f)
						return false // Only add the field once.
					}
				}
				return !currentIsKeyValue // Don't recurse into keyvalues, just look at top level testing.Test fields.
			}, nil)
		}

		return []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  addedUnknownMetadataMsg,
			Link: testRegistrationURL,
		}}
	}

	return issues
}

// verifyLacrosSoftwareDeps verifies that the SoftwareDeps 'lacros_{un}stable' should always be
// defined along with 'lacros'.
// Make sure that 'lacros' is a superset of 'lacros_{un}stable' that represent the breakdown
// {un}stable dependencies.
// It makes it easy to share Lacros tests with the 'dep:lacros' between Chromium and ChromiumOS.
func verifyLacrosSoftwareDeps(fs *token.FileSet, fields entityFields, call *ast.CallExpr) []*Issue {
	// Raise an issue only when 'lacros_{un}stable' is set without 'lacros'.
	if (checkAllSoftwareDeps(fields, "lacros_stable") || checkAllSoftwareDeps(fields, "lacros_unstable")) &&
		!checkAllSoftwareDeps(fields, "lacros") {

		pos := call.Args[0].Pos()
		if s, ok := fields["SoftwareDeps"]; ok {
			pos = s.Pos()
		}

		return []*Issue{{
			Pos: fs.Position(pos),
			Msg: noLacrosSoftwareDepsMsg,
		}}
	}

	return nil
}
