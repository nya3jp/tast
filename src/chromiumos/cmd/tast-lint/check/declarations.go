// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strconv"
	"unicode"
)

// Exposed here for unit tests.
const (
	notTopAddTestMsg = `testing.AddTest() should be called only at a top statement of init()`
	addTestArgLitMsg = `testing.AddTest() should take &testing.Test{...} composite literal`

	noDescMsg         = `Test should have its description`
	nonLiteralDescMsg = `Test descriptions should be string literal`
	badDescMsg        = `Test descriptions should be capitalized phrases without trailing punctuation, e.g. "Checks that foo is bar"`

	noContactMsg          = `Test should list owners' email addresses in Contacts field`
	nonLiteralContactsMsg = `Test Contacts should be an array literal of string literals`

	nonLiteralAttrMsg         = `Test Attr should be an array literal of string literals`
	nonLiteralVarsMsg         = `Test Vars should be an array literal of string literals`
	nonLiteralSoftwareDepsMsg = `Test SoftwareDeps should be an array literal of string literals or (possibly qualified) identifiers`
	nonLiteralParamsMsg       = `Test Params should be an array literal of Param struct literals`
	nonLiteralParamNameMsg    = `Name of Param should be a string literal`

	testRegistrationURL     = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-registration`
	testParamTestURL        = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-test-registration`
	testRuntimeVariablesURL = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Runtime-variables`
)

// Declarations checks declarations of testing.Test structs.
func Declarations(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isEntryFile(filename) {
		return nil
	}

	var issues []*Issue
	for _, decl := range f.Decls {
		issues = append(issues, verifyInit(fs, decl)...)
	}
	return issues
}

// verifyInit checks init() function declared at node.
// If the node is not init() function, returns nil.
func verifyInit(fs *token.FileSet, node ast.Decl) []*Issue {
	decl, ok := node.(*ast.FuncDecl)
	if !ok || decl.Recv != nil || decl.Name.Name != "init" {
		// Not an init() function declaration. Skip.
		return nil
	}

	var issues []*Issue
	for _, stmt := range decl.Body.List {
		issues = append(issues, verifyInitBody(fs, stmt)...)
	}
	return issues
}

// verifyInitBody checks each statement of init()'s body. Specifically
// - testing.AddTest() can be called at a top level statement.
// - testing.AddTest() can take a pointer of a testing.Test composite literal.
// - verifies each element of testing.Test literal.
func verifyInitBody(fs *token.FileSet, stmt ast.Stmt) []*Issue {
	estmt, ok := stmt.(*ast.ExprStmt)
	if !ok || !isTestingAddTestCall(estmt.X) {
		var issues []*Issue
		v := funcVisitor(func(node ast.Node) {
			if isTestingAddTestCall(node) {
				issues = append(issues, &Issue{
					Pos:  fs.Position(node.Pos()),
					Msg:  notTopAddTestMsg,
					Link: testRegistrationURL,
				})
			}
		})
		ast.Walk(v, stmt)
		return issues
	}

	// This is already verified in isTestingAddTestCall().
	call := estmt.X.(*ast.CallExpr)
	if len(call.Args) != 1 {
		// This should be checked by a compiler, so skipped.
		return nil
	}

	// Verify the argument is "&testing.Test{...}"
	arg, ok := call.Args[0].(*ast.UnaryExpr)
	if !ok || arg.Op != token.AND {
		return []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}
	comp, ok := arg.X.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}

	// The compiler should check the type. Skip it.
	var issues []*Issue
	var hasDesc, hasContacts bool
	for _, el := range comp.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch ident.Name {
		case "Desc":
			hasDesc = true
			issues = append(issues, verifyDesc(fs, kv.Value)...)
		case "Contacts":
			hasContacts = true
			issues = append(issues, verifyContacts(fs, kv.Value)...)
		case "Attr":
			issues = append(issues, verifyAttr(fs, kv.Value)...)
		case "Vars":
			issues = append(issues, verifyVars(fs, kv.Value)...)
		case "SoftwareDeps":
			issues = append(issues, verifySoftwareDeps(fs, kv.Value)...)
		case "Params":
			issues = append(issues, verifyParams(fs, kv.Value)...)
		}
	}

	if !hasDesc {
		issues = append(issues, &Issue{
			Pos:  fs.Position(arg.Pos()),
			Msg:  noDescMsg,
			Link: testRegistrationURL,
		})
	}
	if !hasContacts {
		issues = append(issues, &Issue{
			Pos:  fs.Position(arg.Pos()),
			Msg:  noContactMsg,
			Link: testRegistrationURL,
		})
	}
	return issues
}

func verifyDesc(fs *token.FileSet, node ast.Node) []*Issue {
	s, ok := toString(node)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralDescMsg,
			Link: testRegistrationURL,
		}}
	}
	if s == "" || !unicode.IsUpper(rune(s[0])) || s[len(s)-1] == '.' {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  badDescMsg,
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Formatting",
		}}
	}
	return nil
}

func verifyContacts(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralContactsMsg,
			Link: testRegistrationURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		if _, ok := toString(el); !ok {
			issues = append(issues, &Issue{
				Pos:  fs.Position(el.Pos()),
				Msg:  nonLiteralContactsMsg,
				Link: testRegistrationURL,
			})
		}
	}
	return issues
}

func verifyAttr(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralAttrMsg,
			Link: testRegistrationURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		if _, ok := toString(el); !ok {
			issues = append(issues, &Issue{
				Pos:  fs.Position(el.Pos()),
				Msg:  nonLiteralAttrMsg,
				Link: testRegistrationURL,
			})
		}
	}
	return issues
}

func verifyVars(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralVarsMsg,
			Link: testRegistrationURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		if _, ok := toString(el); !ok {
			issues = append(issues, &Issue{
				Pos:  fs.Position(el.Pos()),
				Msg:  nonLiteralVarsMsg,
				Link: testRegistrationURL,
			})
		}
	}
	return issues
}

func verifySoftwareDeps(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralSoftwareDepsMsg,
			Link: testRegistrationURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		if !isStringLiteralOrIdent(el) {
			issues = append(issues, &Issue{
				Pos:  fs.Position(el.Pos()),
				Msg:  nonLiteralSoftwareDepsMsg,
				Link: testRegistrationURL,
			})
		}
	}
	return issues
}

func verifyParams(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralParamsMsg,
			Link: testParamTestURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		issues = append(issues, verifyParamElement(fs, el)...)
	}
	return issues
}

func verifyParamElement(fs *token.FileSet, node ast.Node) []*Issue {
	comp, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralParamsMsg,
			Link: testParamTestURL,
		}}
	}

	var issues []*Issue
	for _, el := range comp.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch ident.Name {
		case "Name":
			if _, ok := toString(kv.Value); !ok {
				issues = append(issues, &Issue{
					Pos:  fs.Position(kv.Value.Pos()),
					Msg:  nonLiteralParamNameMsg,
					Link: testParamTestURL,
				})
			}
		case "ExtraAttr":
			issues = append(issues, verifyAttr(fs, kv.Value)...)
		case "ExtraSoftwareDeps":
			issues = append(issues, verifySoftwareDeps(fs, kv.Value)...)
		}
	}
	return issues
}

// isTestingAddTestCall returns true if the call is an expression
// to invoke testing.AddTest().
func isTestingAddTestCall(node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	return toQualifiedName(call.Fun) == "testing.AddTest"
}

func isStringLiteralOrIdent(node ast.Node) bool {
	if _, ok := toString(node); ok {
		return true
	}
	return toQualifiedName(node) != ""
}

// toString converts the given node representing a string literal
// into string value. If the node is not a string literal, returns
// false for ok.
func toString(node ast.Node) (s string, ok bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}
