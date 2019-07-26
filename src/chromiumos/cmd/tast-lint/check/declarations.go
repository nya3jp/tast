// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// Exposed here for unit tests.
const (
	notTopAddTestMsg    = `testing.AddTest() should be called only at a top statement of init()`
	addTestArgLitMsg    = `testing.AddTest() should take &testing.Test{...} composite literal`
	noContactMsg        = `Test should list owners' email addresses in Contacts field`
	badDescMsg          = `Test descriptions should be capitalized phrases without trailing punctuation, e.g. "Checks that foo is bar"`
	testRegistrationURL = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-registration`
)

// Declarations checks declarations of testing.Test structs.
func Declarations(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isTestMainFile(filename) {
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
	hasContacts := false
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
			issues = append(issues, verifyDesc(fs, kv.Value)...)
		case "Contacts":
			hasContacts = true
		}
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
		// TODO(hidehiko): Make the check more strict that Desc should have string literal.
		return nil
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

// isTestingAddTestCall returns true if the call is an expression
// to invoke testing.AddTest().
func isTestingAddTestCall(node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	return toQualifiedName(call.Fun) == "testing.AddTest"
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

// toQualifiedName stringifies the given node, which is either
// - an ast.Ident node
// - an ast.SelectorExpr node whose .X node is convertible by toQualifiedName.
// If failed, returns an empty string.
func toQualifiedName(node ast.Node) string {
	var comp []string
	for {
		s, ok := node.(*ast.SelectorExpr)
		if !ok {
			break
		}
		comp = append(comp, s.Sel.Name)
		node = s.X
	}

	id, ok := node.(*ast.Ident)
	if !ok {
		return ""
	}
	comp = append(comp, id.Name)

	// Reverse the comp, then join with '.'.
	for i, j := 0, len(comp)-1; i < j; i, j = i+1, j-1 {
		comp[i], comp[j] = comp[j], comp[i]
	}
	return strings.Join(comp, ".")
}
