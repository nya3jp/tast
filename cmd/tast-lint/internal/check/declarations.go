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

	"golang.org/x/tools/go/ast/astutil"

	"go.chromium.org/tast/cmd/tast-lint/internal/git"
)

// Exposed here for unit tests.
const (
	notOnlyTopAddTestMsg = `testing.AddTest() should be the only top level statement of init()`
	addTestArgLitMsg     = `testing.AddTest() should take &testing.Test{...} composite literal`

	noDescMsg         = `Desc field should be filled to describe the registered entity`
	nonLiteralDescMsg = `Desc should be string literal`
	badDescMsg        = `Desc should be capitalized phrases without trailing punctuation, e.g. "Checks that foo is bar"`

	noContactMsg          = `Contacts field should exist to list owners' email addresses`
	nonLiteralContactsMsg = `Contacts field should be an array literal of string literals`

	nonLiteralAttrMsg         = `Test Attr should be an array literal of string literals`
	nonLiteralVarsMsg         = `Test Vars should be an array literal of string literals or constants, or append(array literal, ConstList...)`
	nonLiteralSoftwareDepsMsg = `Test SoftwareDeps should be an array literal of string literals or constants, or append(array literal, ConstList...)`
	nonLiteralParamsMsg       = `Test Params should be an array literal of Param struct literals`
	nonLiteralParamNameMsg    = `Name of Param should be a string literal`

	testRegistrationURL     = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-registration`
	testParamTestURL        = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-test-registration`
	testRuntimeVariablesURL = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Runtime-variables`
)

// TestDeclarations checks declarations of testing.Test structs.
func TestDeclarations(fs *token.FileSet, f *ast.File, path git.CommitFile, fix bool) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isEntryFile(filename) {
		return nil
	}

	var issues []*Issue
	for _, decl := range f.Decls {
		issues = append(issues, verifyInit(fs, decl, path, fix)...)
	}
	return issues
}

// FixtureDeclarations checks declarations of testing.Fixture structs.
func FixtureDeclarations(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	var issues []*Issue
	for _, node := range f.Decls {
		decl, ok := node.(*ast.FuncDecl)
		if !ok || decl.Recv != nil || decl.Name.Name != "init" {
			// Not an init() function declaration. Skip.
			continue
		}
		for _, node := range decl.Body.List {
			expr, ok := node.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := expr.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			if toQualifiedName(call.Fun) != "testing.AddFixture" {
				continue
			}
			issues = append(issues, verifyAddFixtureCall(fs, call, fix)...)
		}
	}
	return issues
}

// verifyInit checks init() function declared at node.
// If the node is not init() function, returns nil.
func verifyInit(fs *token.FileSet, node ast.Decl, path git.CommitFile, fix bool) []*Issue {
	decl, ok := node.(*ast.FuncDecl)
	if !ok || decl.Recv != nil || decl.Name.Name != "init" {
		// Not an init() function declaration. Skip.
		return nil
	}

	if len(decl.Body.List) == 1 {
		if estmt, ok := decl.Body.List[0].(*ast.ExprStmt); ok && isTestingAddTestCall(estmt.X) {
			// X's type is already verified in isTestingAddTestCall().
			return verifyAddTestCall(fs, estmt.X.(*ast.CallExpr), path, fix)
		}
	}

	var addTestNode ast.Node
	ast.Walk(funcVisitor(func(n ast.Node) {
		if addTestNode == nil && isTestingAddTestCall(n) {
			addTestNode = n
		}
	}), node)

	if addTestNode != nil {
		return []*Issue{{
			Pos:  fs.Position(addTestNode.Pos()),
			Msg:  notOnlyTopAddTestMsg,
			Link: testRegistrationURL,
		}}
	}
	return nil
}

type entityFields map[string]*ast.KeyValueExpr

// registeredEntityFields returns a mapping from field name to value, or issues
// on error.
// call must be a registration of an entity, e.g. testing.AddTest or
// testing.AddFixture.
func registeredEntityFields(fs *token.FileSet, call *ast.CallExpr) (entityFields, []*Issue) {
	if len(call.Args) != 1 {
		// This should be checked by a compiler, so skipped.
		return nil, nil
	}

	// Verify the argument is "&testing.Test{...}"
	arg, ok := call.Args[0].(*ast.UnaryExpr)
	if !ok || arg.Op != token.AND {
		return nil, []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}
	comp, ok := arg.X.(*ast.CompositeLit)
	if !ok {
		return nil, []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}

	res := make(entityFields)
	for _, el := range comp.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		res[ident.Name] = kv
	}
	return res, nil
}

func verifyAddFixtureCall(fs *token.FileSet, call *ast.CallExpr, fix bool) []*Issue {
	fields, issues := registeredEntityFields(fs, call)
	if len(issues) > 0 {
		return issues
	}
	issues = append(issues, verifyVars(fs, fields)...)
	issues = append(issues, verifyDesc(fs, fields, call, fix)...)
	issues = append(issues, verifyContacts(fs, fields, call)...)
	return issues
}

// verifyAddTestCall verifies testing.AddTest calls. Specifically
// - testing.AddTest() can take a pointer of a testing.Test composite literal.
// - verifies each element of testing.Test literal.
func verifyAddTestCall(fs *token.FileSet, call *ast.CallExpr, path git.CommitFile, fix bool) []*Issue {
	fields, issues := registeredEntityFields(fs, call)
	if len(issues) > 0 {
		return issues
	}

	if kv, ok := fields["Attr"]; ok {
		issues = append(issues, verifyAttr(fs, kv.Value)...)
	}
	if kv, ok := fields["SoftwareDeps"]; ok {
		issues = append(issues, verifySoftwareDeps(fs, kv.Value)...)
	}
	issues = append(issues, verifyVars(fs, fields)...)
	issues = append(issues, verifyParams(fs, fields)...)
	issues = append(issues, verifyDesc(fs, fields, call, fix)...)
	issues = append(issues, verifyContacts(fs, fields, call)...)
	issues = append(issues, verifyLacrosStatus(fs, fields, path, call, fix)...)

	return issues
}

func verifyDesc(fs *token.FileSet, fields entityFields, call *ast.CallExpr, fix bool) []*Issue {
	kv, ok := fields["Desc"]
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  noDescMsg,
			Link: testRegistrationURL,
		}}
	}
	node := kv.Value
	s, ok := toString(node)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralDescMsg,
			Link: testRegistrationURL,
		}}
	}
	if s == "" || !unicode.IsUpper(rune(s[0])) || s[len(s)-1] == '.' {
		if !fix {
			return []*Issue{{
				Pos:     fs.Position(node.Pos()),
				Msg:     badDescMsg,
				Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Formatting",
				Fixable: true,
			}}
		}
		astutil.Apply(kv, func(c *astutil.Cursor) bool {
			lit, ok := c.Node().(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strtype, ok := stringLitTypeOf(lit.Value); ok {
				c.Replace(&ast.BasicLit{
					Kind:  token.STRING,
					Value: quoteAs(strings.TrimRight(strings.ToUpper(s[:1])+s[1:], "."), strtype),
				})
			}
			return false
		}, nil)
	}
	return nil
}

func verifyContacts(fs *token.FileSet, fields entityFields, call *ast.CallExpr) []*Issue {
	kv, ok := fields["Contacts"]
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(call.Args[0].Pos()),
			Msg:  noContactMsg,
			Link: testRegistrationURL,
		}}
	}

	comp, ok := kv.Value.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(kv.Value.Pos()),
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

func isStaticString(expr ast.Expr) bool {
	_, isString := expr.(*ast.BasicLit)
	_, isIdent := expr.(*ast.Ident)
	_, isSelector := expr.(*ast.SelectorExpr)
	return isString || isSelector || isIdent
}

func isStaticStringList(expr ast.Expr) bool {
	_, isSelector := expr.(*ast.SelectorExpr)
	if isSelector {
		return true
	}
	if compositeLit, ok := expr.(*ast.CompositeLit); ok {
		for _, arg := range compositeLit.Elts {
			if !isStaticString(arg) {
				return false
			}
		}
		return true
	}

	if callExpr, ok := expr.(*ast.CallExpr); ok {
		fun, ok := callExpr.Fun.(*ast.Ident)
		if !ok || fun.Name != "append" {
			return false
		}
		for i, arg := range callExpr.Args {
			isVarList := i == 0 || (i == len(callExpr.Args)-1 && callExpr.Ellipsis != token.NoPos)
			if isVarList && !isStaticStringList(arg) {
				return false
			}
			if !isVarList && !isStaticString(arg) {
				return false
			}
		}
		return true
	}
	// Since the type of the expression is a list, any selector must be a list constant.
	_, ok := expr.(*ast.SelectorExpr)
	return ok
}

func verifyVars(fs *token.FileSet, fields entityFields) []*Issue {
	kv, ok := fields["Vars"]
	if !ok {
		return nil
	}

	if !isStaticStringList(kv.Value) {
		return []*Issue{{
			Pos:  fs.Position(kv.Value.Pos()),
			Msg:  nonLiteralVarsMsg,
			Link: testRegistrationURL,
		}}
	}
	return nil
}

func verifySoftwareDeps(fs *token.FileSet, node ast.Expr) []*Issue {
	if !isStaticStringList(node) {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralSoftwareDepsMsg,
			Link: testRegistrationURL,
		}}
	}
	return nil
}

func verifyParams(fs *token.FileSet, fields entityFields) []*Issue {
	kv, ok := fields["Params"]
	if !ok {
		return nil
	}

	comp, ok := kv.Value.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(kv.Value.Pos()),
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
