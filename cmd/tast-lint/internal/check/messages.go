// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// countVerbs returns the number of format verbs.
func countVerbs(s string) int {
	// "verb" starts with '%', but exclude '%%'s.
	return strings.Count(s, "%") - strings.Count(s, "%%")*2
}

// printFormatRE matches format verbs.
var printFormatRE = regexp.MustCompile(`%[#+]?\d*\.?\d*[dfqsvx]`)

// invalidQuotedVerbs matches "%s", '%s', "%v" and '%v'.
var invalidQuotedVerbs = regexp.MustCompile(`("%[sv]")|('%[sv]')`)

// argType represents argument type as enumerated value.
type argType int

const (
	stringArg argType = iota
	errorArg
	otherArg
)

// argInfo holds type, value and node of the argument.
type argInfo struct {
	typ   argType
	val   string
	fixed bool
	node  ast.Expr
}

// hasVerbs returns if s contains format verbs.
func hasVerbs(s string) bool {
	return printFormatRE.MatchString(s)
}

func isFMTSprintf(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return x.Name == "fmt" && sel.Sel.Name == "Sprintf"
}

const (
	baseURL       = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md"
	printfURL     = baseURL + "#log-vs-logf"
	errPkgURL     = baseURL + "#error-pkg"
	errFmtURL     = baseURL + "#error-fmt"
	errConstURL   = baseURL + "#error-construction"
	logFmtURL     = baseURL + "#log-fmt"
	commonFmtURL  = baseURL + "#common-fmt"
	formattingURL = baseURL + "#Formatting"
	fmtURL        = "https://golang.org/pkg/fmt/#hdr-Printing"
)

// fmtMap holds f-suffixed functions as keys and their values are corresponding non-f-suffixed functions.
var fmtMap = map[string]string{
	"s.Logf":              "s.Log",
	"s.Errorf":            "s.Error",
	"s.Fatalf":            "s.Fatal",
	"testing.ContextLogf": "testing.ContextLog",
	"errors.Errorf":       "errors.New",
	"errors.Wrapf":        "errors.Wrap",
}

// fmtMapRev is reversed map of fmtMap.
var fmtMapRev = map[string]string{}

func init() {
	for k, v := range fmtMap {
		fmtMapRev[v] = k
	}
}

// Messages checks calls to logging- and error-related functions.
func Messages(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	// Handle Sprintf cases beforehand.
	issues := messagesSprintf(fs, f, fix)

	astutil.Apply(f, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}

		recvName, funcName, callName, argOffset, argPrefix, ok := messagesInfo(call)
		if !ok {
			return true
		}

		_, isFmt := fmtMap[callName]
		isErr := recvName == "errors"
		isLog := strings.HasPrefix(callName, "s.Log") || strings.HasPrefix(callName, "testing.ContextLog")

		// preArgs keeps the arguments before the formatting string.
		var preArgs []argInfo
		for _, a := range call.Args[:argOffset] {
			preArgs = append(preArgs, argInfo{otherArg, "", false, a})
		}

		var args []argInfo
		for _, a := range call.Args[argOffset:] {
			if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				args = append(args, argInfo{stringArg, val, false, lit})
			} else if ident, ok := a.(*ast.Ident); ok && ident.Name == "err" {
				// Again, checking the identifier name is unreliable,
				// but getting the actual type here seems hard/impossible.
				// This means that we'll miss error values with other names.
				args = append(args, argInfo{errorArg, "", false, ident})
			} else {
				args = append(args, argInfo{otherArg, "", false, a})
			}
		}

		addIssue := func(msg, link string, fixable bool) {
			issues = append(issues, &Issue{Pos: fs.Position(call.Pos()), Msg: msg, Link: link, Fixable: fixable})
		}

		// Used Logf("Some message") instead of Log("Some message").
		if isFmt && len(args) == 1 {
			// If there is a verb in the string argument, there is a possibility of
			// missing other arguments. If so, define as unfixable and leave as it is.
			fixable := (args[0].typ != stringArg || countVerbs(args[0].val) == 0)
			if !fix {
				addIssue(fmt.Sprintf(`Use %v(%v"<msg>") instead of %v(%v"<msg>")`,
					fmtMap[callName], argPrefix, callName, argPrefix), printfURL, fixable)
			} else if fixable {
				callName = fmtMap[callName]
				funcName = strings.TrimPrefix(callName, recvName+".")
				isFmt = false
			}
		}

		// Used Logf("Got %v", i) instead of Log("Got ", i).
		if !isErr && isFmt && len(args) == 2 && args[0].typ == stringArg && strings.HasSuffix(args[0].val, " %v") {
			if !fix {
				addIssue(fmt.Sprintf(`Use %v(%v"<msg> ", val) instead of %v(%v"<msg> %%v", val)`,
					fmtMap[callName], argPrefix, callName, argPrefix), printfURL, true)
			} else {
				args[0].val = strings.TrimSuffix(args[0].val, "%v")
				args[0].fixed = true
				callName = fmtMap[callName]
				funcName = strings.TrimPrefix(callName, recvName+".")
				isFmt = false
			}
		}

		// Used Log("Some error", err) instead of Log("Some error: ", err).
		if !isFmt && !isErr && len(args) == 2 && args[0].typ == stringArg &&
			args[1].typ == errorArg && !strings.HasSuffix(args[0].val, ": ") {
			if !fix {
				addIssue(fmt.Sprintf(`%v string arg should end with ": " when followed by error`, callName), logFmtURL, true)
			} else {
				// If there are trailing punctuations, it will be checked by lint below,
				// so remove them before adding colon and space.
				// Just a colon or a space should be removed before adding colon and space too.
				args[0].val = strings.TrimRight(args[0].val, ".!: ") + ": "
				args[0].fixed = true
			}
		}

		// Used errors.Errorf("something failed: %v", err) instead of errors.Wrap(err, "something failed").
		if callName == "errors.Errorf" && len(args) >= 2 && args[0].typ == stringArg &&
			args[len(args)-1].typ == errorArg && strings.HasSuffix(args[0].val, "%v") {
			if !fix {
				var formatSuffix, varArgs string
				if len(args) > 2 {
					formatSuffix, varArgs = "f", ", ..."
				}
				addIssue(fmt.Sprintf(`Use errors.Wrap%s(err, "<msg>"%s) instead of errors.Errorf("<msg>: %%v"%s, err)`, formatSuffix, varArgs, varArgs), errPkgURL, true)
			} else {
				funcName = "Wrapf"
				if len(args) == 2 {
					funcName = "Wrap"
					isFmt = false
				}
				callName = recvName + "." + funcName
				args[0].val = strings.TrimRight(strings.TrimSuffix(args[0].val, "%v"), ".!: ")
				args[0].fixed = true
				lastArg := args[len(args)-1]
				args = args[:len(args)-1]
				// Position information may affect the order of expression nodes,
				// so clear it.
				lastArg.node.(*ast.Ident).NamePos = token.NoPos
				preArgs = append(preArgs, lastArg)
			}
		}

		// Used errors.Wrap(err, "something failed: ") instead of errors.Wrap(err, "something failed").
		if callName == "errors.Wrap" && len(args) == 1 && args[0].typ == stringArg &&
			strings.HasSuffix(strings.TrimSpace(args[0].val), ":") {
			messageTrimmed := strings.TrimSuffix(strings.TrimSpace(args[0].val), ":")
			if !fix {
				addIssue(fmt.Sprintf(`Use errors.Wrap(err, "%v") instead of errors.Wrap(err, "%v")`,
					messageTrimmed, args[0].val), errPkgURL, true)
			} else {
				args[0].val = messageTrimmed
				args[0].fixed = true
			}
		}

		// Used errors.Wrapf(err, "something failed: %v", err) instead of errors.Wrap(err, "something failed")
		if callName == "errors.Wrapf" {
			for _, e := range args {
				if e.typ == errorArg {
					addIssue(`Use errors.Wrap(err, "<msg>") instead of errors.Wrapf(err, "<msg>: %v", err)`, errConstURL, true)
					break
				}
			}
		}

		// Used Log(err) instead of Log("Some error: ", err).
		if !isErr && len(args) == 1 && args[0].typ == errorArg {
			addIssue(fmt.Sprintf(`Use %v(%v"Something failed: ", err) instead of %v(%verr)`,
				callName, argPrefix, callName, argPrefix), commonFmtURL, false)
		}

		// Lower-level string checks.
		if len(args) >= 1 && args[0].typ == stringArg && args[0].val != "" {
			// Used Log("Some message.") or Log("Some message!") instead of Log("Some message").
			if strings.LastIndexAny(args[0].val, ".!") == len(args[0].val)-1 {
				if !fix {
					u := logFmtURL
					if isErr {
						u = errFmtURL
					}
					addIssue(fmt.Sprintf("%v string arg should not contain trailing punctuation", callName), u, true)
				} else {
					args[0].val = strings.TrimRight(args[0].val, ".!")
					args[0].fixed = true
				}
			}
			// Used Log("Some message\nMore text") instead of Log("Some message") and Log("More text").
			if strings.Contains(args[0].val, "\n") {
				addIssue(fmt.Sprintf("%v string arg should not contain embedded newlines", callName), commonFmtURL, false)
			}
			// Used Logf("'%s'", ...) instead of Logf("%q", ...).
			if isFmt && invalidQuotedVerbs.MatchString(args[0].val) {
				if !fix {
					addIssue("Use %q to quote values instead of manually quoting them", commonFmtURL, true)
				} else {
					args[0].val = invalidQuotedVerbs.ReplaceAllString(args[0].val, "%q")
					args[0].fixed = true
				}
			}

			// Logs should start with upper letters while errors should start with lower letters.
			// This rule does not apply if messages start with a proper noun, so we check a few
			// hard-coded words only.
			var words = []string{
				"bad",
				"can",
				"can't",
				"cannot",
				"could",
				"couldn't",
				"didn't",
				"error",
				"expected",
				"failed",
				"found",
				"get",
				"getting",
				"got",
				"invalid",
				"no",
				"the",
				"too",
				"unexpected",
				"unknown",
				"unsupported",
			}

			for _, word := range words {
				if !strings.HasPrefix(strings.ToLower(args[0].val), word+" ") {
					continue
				}
				exp := word
				if !isErr {
					exp = strings.ToUpper(word[:1]) + word[1:]
				}
				if strings.HasPrefix(args[0].val, exp) {
					continue
				}
				if !fix {
					if isErr {
						addIssue("Messages of the error type should not be capitalized", formattingURL, true)
					} else if isLog {
						addIssue("Log messages should be capitalized", formattingURL, true)
					} else {
						addIssue("Test failure messages should be capitalized", formattingURL, true)
					}
				} else {
					args[0].val = exp + args[0].val[len(exp):]
					args[0].fixed = true
				}
			}
		}

		// The number of verbs and the number of arguments must match.
		if isFmt && len(args) >= 1 && args[0].typ == stringArg && countVerbs(args[0].val) != len(args)-1 {
			addIssue("The number of verbs in format literal mismatches with the number of arguments", fmtURL, false)
		}
		// Used verbs in non *f families.
		if !isFmt && len(args) >= 1 && args[0].typ == stringArg && hasVerbs(args[0].val) {
			addIssue(fmt.Sprintf("%s has verbs in the first string (do you mean %s?)", callName, fmtMapRev[callName]), formattingURL, false)
		}

		// Error messages should contain some surrounding context.
		if !isLog && len(args) >= 1 && args[0].typ == stringArg && args[0].val == "" {
			addIssue("Error message should have some surrounding context, so must not empty", errFmtURL, false)
		}

		if fix {
			newarg := append(makeCallArgs(preArgs), makeCallArgs(args)...)
			correct := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.Ident{
						Name:    recvName,
						NamePos: call.Pos(),
					},
					Sel: &ast.Ident{
						Name: funcName,
					},
				},
				Args: newarg,
			}
			c.Replace(correct)
		}

		return true
	}, nil)

	return issues
}

// messagesSprintf checks calls to logging- and error-related functions
// which has fmt.Sprintf argument inside them.
func messagesSprintf(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	var issues []*Issue

	astutil.Apply(f, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		recvName, funcName, callName, argOffset, argPrefix, ok := messagesInfo(call)
		if !ok {
			return true
		}

		_, isFmt := fmtMap[callName]

		// Used Log(fmt.Sprintf(...)) instead of Logf(...)
		if argcall, ok := call.Args[argOffset].(*ast.CallExpr); ok && isFMTSprintf(argcall) && len(call.Args)-argOffset == 1 {
			if !fix {
				msg := ""
				if isFmt {
					msg = fmt.Sprintf(`Use %v(%v...) instead of %v(%vfmt.Sprintf(...))`, callName, argPrefix, callName, argPrefix)
				} else {
					msg = fmt.Sprintf(`Use %v(%v...) instead of %v(%vfmt.Sprintf(...))`, fmtMapRev[callName], argPrefix, callName, argPrefix)
				}
				issues = append(issues, &Issue{
					Pos:     fs.Position(call.Pos()),
					Msg:     msg,
					Link:    printfURL,
					Fixable: true,
				})
			} else {
				newarg := append([]ast.Expr(nil), call.Args[:argOffset]...)
				newarg = append(newarg, argcall.Args...)
				sel := funcName
				if !isFmt {
					sel = strings.TrimPrefix(fmtMapRev[callName], recvName+".")
				}
				correct := &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.Ident{
							Name:    recvName,
							NamePos: call.Pos(),
						},
						Sel: &ast.Ident{
							Name: sel,
						},
					},
					Args: newarg,
				}
				c.Replace(correct)
			}
		}

		return true
	}, nil)

	return issues
}

// messagesInfo returns function name and argument information.
func messagesInfo(call *ast.CallExpr) (recvName, funcName, callName string, argOffset int, argPrefix string, ok bool) {
	argOffset = -1
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	recvName = x.Name
	funcName = sel.Sel.Name
	callName = recvName + "." + funcName

	switch callName {
	// Just matching a variable name is hacky, but getting the actual type seems
	// very challenging, if not impossible (as ":=" may be used to assign types
	// returned by functions exported by other packages).
	case "s.Log", "s.Logf", "s.Error", "s.Errorf", "s.Fatal", "s.Fatalf", "errors.New", "errors.Errorf":
		argOffset = 0
	case "testing.ContextLog", "testing.ContextLogf":
		argOffset = 1
		argPrefix = "ctx, "
	case "errors.Wrap", "errors.Wrapf":
		argOffset = 1
		argPrefix = "err, "
	default:
		ok = false
	}

	if len(call.Args) <= argOffset {
		ok = false
	}

	return
}

// makeCallArgs makes slices of expression nodes with argInfo values.
func makeCallArgs(args []argInfo) []ast.Expr {
	var callargs []ast.Expr

	for _, a := range args {
		if a.typ == stringArg {
			if strtype, ok := stringLitTypeOf(a.node.(*ast.BasicLit).Value); ok && a.fixed {
				a.node.(*ast.BasicLit).Value = quoteAs(a.val, strtype)
			}
		}
		callargs = append(callargs, a.node)
	}

	return callargs
}
