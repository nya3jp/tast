// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"golang.org/x/tools/go/ast/astutil"
)

// Exposed here for unit tests.
const (
	notOnlyTopAddTestMsg = `testing.AddTest() should be the only top level statement of init()`
	addTestArgLitMsg     = `testing.AddTest() should take &testing.Test{...} composite literal`

	noDescMsg         = `Test should have its description`
	nonLiteralDescMsg = `Test descriptions should be string literal`
	badDescMsg        = `Test descriptions should be capitalized phrases without trailing punctuation, e.g. "Checks that foo is bar"`

	noContactMsg          = `Test should list owners' email addresses in Contacts field`
	nonLiteralContactsMsg = `Test Contacts should be an array literal of string literals`

	nonLiteralAttrMsg         = `Test Attr should be an array literal of string literals`
	nonLiteralVarsMsg         = `Test Vars should be an array literal of string literals or constants`
	nonLiteralSoftwareDepsMsg = `Test SoftwareDeps should be an array literal of string literals or (possibly qualified) identifiers`
	nonLiteralParamsMsg       = `Test Params should be an array literal of Param struct literals`
	nonLiteralParamNameMsg    = `Name of Param should be a string literal`

	testRegistrationURL     = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Test-registration`
	testParamTestURL        = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-test-registration`
	testRuntimeVariablesURL = `https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Runtime-variables`
)

var knownRequiredVars = []string{
	"arc.OptInAfterInterruption.unmanaged_username",
	"arc.OptInAfterInterruption.unmanaged_password",
	"arc.OptInAfterInterruption.managed_username",
	"arc.OptInAfterInterruption.managed_password",

	"enterprise.ARCProvisioning.user",
	"enterprise.ARCProvisioning.password",
	"enterprise.ARCProvisioning.packages",
	"enterprise.ARCProvisioning.necktie_user",
	"enterprise.ARCProvisioning.necktie_password",
	"enterprise.ARCProvisioning.necktie_packages",
	"enterprise.ARCProvisioning.unmanaged_user",
	"enterprise.ARCProvisioning.unmanaged_password",
	"enterprise.ARCProvisioning.unmanaged_packages",
	"filemanager.user",
	"filemanager.password",
	"arcappcompat.username",
	"arcappcompat.password",
	"arcappcompat.Hearthstone.username",
	"arcappcompat.Hearthstone.password",
	"arcappcompat.Noteshelf.username",
	"arcappcompat.Noteshelf.password",
	"arcappcompat.Photolemur.username",
	"arcappcompat.Photolemur.password",
	"arcappcompat.MyscriptNebo.username",
	"arcappcompat.MyscriptNebo.password",
	"arcappcompat.Artrage.username",
	"arcappcompat.Artrage.password",
	"arcappcompat.CrossDJ.username",
	"arcappcompat.CrossDJ.password",
	"arc.AppLoadingPerf.username",
	"arc.AppLoadingPerf.password",

	"arc.AuthPerf.unmanaged_username",
	"arc.AuthPerf.unmanaged_password",
	"arc.AuthPerf.managed_username",
	"arc.AuthPerf.managed_password",

	"arc.EnterpriseLogin.managed_3pp_true_user",
	"arc.EnterpriseLogin.managed_3pp_true_password",
	"arc.EnterpriseLogin.managed_3pp_false_user",
	"arc.EnterpriseLogin.managed_3pp_false_password",
	"arc.EnterpriseLogin.managed_necktie_false_user",
	"arc.EnterpriseLogin.managed_necktie_false_password",
	"arc.EnterpriseLogin.managed_necktie_true_user",
	"arc.EnterpriseLogin.managed_necktie_true_password",
	"arc.EnterpriseLogin.managed_unmanaged_false_user",
	"arc.EnterpriseLogin.managed_unmanaged_false_password",
	"arc.EnterpriseLogin.managed_unmanaged_true_user",
	"arc.EnterpriseLogin.managed_unmanaged_true_password",

	"arc.OptInAfterInterruption.unmanaged_username",
	"arc.OptInAfterInterruption.unmanaged_password",
	"arc.OptInAfterInterruption.managed_username",
	"arc.OptInAfterInterruption.managed_password",

	"enterprise.ARCProvisioning.user",
	"enterprise.ARCProvisioning.password",
	"enterprise.ARCProvisioning.packages",
	"enterprise.ARCProvisioning.necktie_user",
	"enterprise.ARCProvisioning.necktie_password",
	"enterprise.ARCProvisioning.necktie_packages",
	"enterprise.ARCProvisioning.unmanaged_user",
	"enterprise.ARCProvisioning.unmanaged_password",
	"enterprise.ARCProvisioning.unmanaged_packages",

	"filemanager.drive_credentials",
	"ui.gaiaPoolDefault",

	"servo",
}

// Declarations checks declarations of testing.Test structs.
func Declarations(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isEntryFile(filename) {
		return nil
	}

	reqVars := requiredVars(f)

	var issues []*Issue
	for _, decl := range f.Decls {
		issues = append(issues, verifyInit(fs, decl, fix, reqVars)...)
	}
	return issues
}

// Declarations2 update Vars to VarDeps.
func Declarations2(fs *token.FileSet, f *ast.File, fix bool) (*token.FileSet, *ast.File, []*Issue) {
	filename := fs.Position(f.Package).Filename
	if !isEntryFile(filename) {
		return fs, f, nil
	}

	df, err := decorator.DecorateFile(fs, f)
	if err != nil {
		panic(err)
	}

	reqVars := requiredVars(f)

	var issues []*Issue
	for _, decl := range df.Decls {
		issues = append(issues, verifyInit2(fs, decl, fix, reqVars)...)
	}

	fs, f, err = decorator.RestoreFile(df)
	if err != nil {
		panic(err)
	}

	return fs, f, issues
}

func requiredVars(f *ast.File) map[string]bool {
	res := make(map[string]bool)

	for _, v := range knownRequiredVars {
		res[`"`+v+`"`] = true
	}

	for _, node := range f.Decls {
		decl, ok := node.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Walk(funcVisitor(func(n ast.Node) {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return
			}
			if toQualifiedName(call.Fun) != "s.RequiredVar" {
				return
			}
			s, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				// TODO(oka): process non-literals
				return
			}
			res[s.Value] = true
		}), decl.Body)
	}
	return res
}

// verifyInit checks init() function declared at node.
// If the node is not init() function, returns nil.
func verifyInit(fs *token.FileSet, node ast.Decl, fix bool, reqVars map[string]bool) []*Issue {
	decl, ok := node.(*ast.FuncDecl)
	if !ok || decl.Recv != nil || decl.Name.Name != "init" {
		// Not an init() function declaration. Skip.
		return nil
	}

	if len(decl.Body.List) == 1 {
		if estmt, ok := decl.Body.List[0].(*ast.ExprStmt); ok && isTestingAddTestCall(estmt.X) {
			// X's type is already verified in isTestingAddTestCall().
			return verifyAddTestCall(fs, estmt.X.(*ast.CallExpr), fix, reqVars)
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

func verifyInit2(fs *token.FileSet, node dst.Decl, fix bool, reqVars map[string]bool) []*Issue {
	decl, ok := node.(*dst.FuncDecl)
	if !ok || decl.Recv != nil || decl.Name.Name != "init" {
		// Not an init() function declaration. Skip.
		return nil
	}

	if len(decl.Body.List) == 1 {
		if estmt, ok := decl.Body.List[0].(*dst.ExprStmt); ok && isTestingAddTestCall2(estmt.X) {
			// X's type is already verified in isTestingAddTestCall().
			return verifyAddTestCall2(estmt.X.(*dst.CallExpr), fix, reqVars)
		}
	}
	return nil
}

// verifyAddTestCall verifies testing.AddTest calls. Specifically
// - testing.AddTest() can take a pointer of a testing.Test composite literal.
// - verifies each element of testing.Test literal.
func verifyAddTestCall(fs *token.FileSet, call *ast.CallExpr, fix bool, reqVars map[string]bool) []*Issue {
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
			issues = append(issues, verifyDesc(fs, kv, fix)...)
		case "Contacts":
			hasContacts = true
			issues = append(issues, verifyContacts(fs, kv.Value)...)
		case "Attr":
			issues = append(issues, verifyAttr(fs, kv.Value)...)
		case "Vars":
			issues = append(issues, verifyVars(fs, kv.Value, reqVars)...)
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

// verifyAddTestCall verifies testing.AddTest calls. Specifically
// - testing.AddTest() can take a pointer of a testing.Test composite literal.
// - verifies each element of testing.Test literal.
func verifyAddTestCall2(call *dst.CallExpr, fix bool, reqVars map[string]bool) []*Issue {
	if len(call.Args) != 1 {
		// This should be checked by a compiler, so skipped.
		return nil
	}

	// Verify the argument is "&testing.Test{...}"
	arg, ok := call.Args[0].(*dst.UnaryExpr)
	if !ok || arg.Op != token.AND {
		return []*Issue{{
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}
	comp, ok := arg.X.(*dst.CompositeLit)
	if !ok {
		return []*Issue{{
			Msg:  addTestArgLitMsg,
			Link: testRegistrationURL,
		}}
	}

	// The compiler should check the type. Skip it.
	var issues []*Issue

	varsID := -1
	varDepsID := -1
	for i, el := range comp.Elts {
		kv, ok := el.(*dst.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*dst.Ident)
		if !ok {
			continue
		}
		switch ident.Name {
		case "Vars":
			varsID = i
		case "VarDeps":
			varDepsID = i
		}
	}

	issues = append(issues, verifyVars2(reqVars, comp, varsID, varDepsID, fix)...)

	return issues
}

// verifyVars verifies runtime variables declarations.
// Negative ID indicates corresponding field doesn't exist.
func verifyVars2(reqVars map[string]bool, comp *dst.CompositeLit, varsID, varDepsID int, fix bool) []*Issue {
	var issues []*Issue
	for _, i := range []int{varsID, varDepsID} {
		if i < 0 {
			continue
		}
		value := comp.Elts[i].(*dst.KeyValueExpr).Value
		_, ok := value.(*dst.CompositeLit)

		if !ok {
			issues = append(issues, &Issue{
				// Pos:  fs.Position(value.Pos()),
				Msg:  nonLiteralVarsMsg,
				Link: testRegistrationURL,
			})
		}
	}
	if len(issues) > 0 || varsID < 0 {
		return issues
	}

	var toVarDeps []dst.Expr
	removeIDs := make(map[int]bool)
	varElts := &comp.Elts[varsID].(*dst.KeyValueExpr).Value.(*dst.CompositeLit).Elts
	for i, n := range *varElts {
		s, ok := n.(*dst.BasicLit)
		if !ok {
			continue
		}
		if !reqVars[s.Value] {
			continue
		}
		if fix {
			toVarDeps = append(toVarDeps, n)
			removeIDs[i] = true
		} else {
			issues = append(issues, &Issue{
				// Pos:     fs.Position(n.Pos()),
				Msg:     fmt.Sprintf("%s is used in s.RequiredVar; use VarDeps for registration", s.Value),
				Link:    testRuntimeVariablesURL,
				Fixable: true,
			})
		}
	}

	if len(toVarDeps) == 0 {
		return issues
	}
	if len(*varElts) == len(toVarDeps) && varDepsID < 0 {
		comp.Elts[varsID].(*dst.KeyValueExpr).Key.(*dst.Ident).Name = "VarDeps"
		return nil
	}

	if varDepsID < 0 {
		// insert
		comp.Elts = append(append(append([]dst.Expr(nil), comp.Elts[0:varsID+1]...),
			&dst.KeyValueExpr{
				Key: dst.NewIdent("VarDeps"),
				Value: &dst.CompositeLit{
					Type: dst.NewIdent("[]string"),
				},
				Decs: dst.KeyValueExprDecorations{
					NodeDecs: dst.NodeDecs{
						End: []string{"\n"},
					},
				},
			}), comp.Elts[varsID+1:]...)
		varDepsID = varsID + 1
	}

	comp.Elts[varDepsID].(*dst.KeyValueExpr).Value.(*dst.CompositeLit).Elts = append(
		comp.Elts[varDepsID].(*dst.KeyValueExpr).Value.(*dst.CompositeLit).Elts, toVarDeps...)

	// Remove toVarDeps from VarDeps.
	var newElts []dst.Expr
	for i, n := range *varElts {
		if removeIDs[i] {
			continue
		}
		newElts = append(newElts, n)
	}
	*varElts = newElts
	if len(newElts) == 0 {
		comp.Elts = append(append([]dst.Expr(nil), comp.Elts[:varsID]...), comp.Elts[varsID+1:]...)
	}
	return nil
}

func verifyDesc(fs *token.FileSet, kv *ast.KeyValueExpr, fix bool) []*Issue {
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

func verifyVars(fs *token.FileSet, node ast.Node, reqVars map[string]bool) []*Issue {
	array, ok := node.(*ast.CompositeLit)
	if !ok {
		return []*Issue{{
			Pos:  fs.Position(node.Pos()),
			Msg:  nonLiteralVarsMsg,
			Link: testRegistrationURL,
		}}
	}
	var issues []*Issue
	for _, n := range array.Elts {
		s, ok := n.(*ast.BasicLit)
		if !ok {
			continue
		}
		if !reqVars[s.Value] {
			continue
		}
		issues = append(issues, &Issue{
			Pos:  fs.Position(n.Pos()),
			Msg:  fmt.Sprintf("%s is used in s.RequiredVar; use VarDeps for registration", s.Value),
			Link: testRuntimeVariablesURL,
		})
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

// isTestingAddTestCall returns true if the call is an expression
// to invoke testing.AddTest().
func isTestingAddTestCall2(node dst.Node) bool {
	call, ok := node.(*dst.CallExpr)
	if !ok {
		return false
	}
	return toQualifiedName2(call.Fun) == "testing.AddTest"
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
