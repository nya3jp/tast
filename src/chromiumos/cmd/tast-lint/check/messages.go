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
)

// countVerbs returns the number of format verbs.
func countVerbs(s string) int {
	// "verb" starts with '%', but exclude '%%'s.
	return strings.Count(s, "%") - strings.Count(s, "%%")*2
}

// printFormatRE matches format verbs.
var printFormatRE = regexp.MustCompile(`%[#+]?\d*\.?\d*[dfqsvx]`)

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

// Messages checks calls to logging- and error-related functions.
func Messages(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return
		}
		recvName := x.Name
		funcName := sel.Sel.Name
		callName := recvName + "." + funcName

		argOffset := -1 // index of first interesting arg in call.Args
		argPrefix := "" // uninteresting args, e.g. "ctx, " or "err, "
		switch callName {
		// Just matching a variable name is hacky, but getting the actual type seems
		// very challenging, if not impossible (as ":=" may be used to assign types
		// returned by functions exported by other packages).
		case "s.Log", "s.Logf", "s.Error", "s.Errorf", "s.Fatal", "s.Fatalf":
			argOffset = 0
		case "testing.ContextLog", "testing.ContextLogf":
			argOffset = 1
			argPrefix = "ctx, "
		case "errors.New", "errors.Errorf":
			argOffset = 0
		case "errors.Wrap", "errors.Wrapf":
			argOffset = 1
			argPrefix = "err, "
		}

		if argOffset < 0 || len(call.Args) <= argOffset {
			return
		}

		// Keys are f-suffixed functions, values are corresponding non-f-suffixed functions.
		var fmtMap = map[string]string{
			"s.Logf":              "s.Log",
			"s.Errorf":            "s.Error",
			"s.Fatalf":            "s.Fatal",
			"testing.ContextLogf": "testing.ContextLog",
			"errors.Errorf":       "errors.New",
			"errors.Wrapf":        "errors.Wrap",
		}
		fmtMapRev := make(map[string]string)
		for k, v := range fmtMap {
			fmtMapRev[v] = k
		}

		_, isFmt := fmtMap[callName]
		isErr := recvName == "errors"
		isLog := strings.HasPrefix(callName, "s.Log") || strings.HasPrefix(callName, "testing.ContextLog")

		type argType int
		const (
			stringArg argType = iota
			sprintfArg
			errorArg
			otherArg
		)
		type argInfo struct {
			typ argType
			val string
		}
		var args []argInfo
		for _, a := range call.Args[argOffset:] {
			if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					return
				}
				args = append(args, argInfo{stringArg, val})
			} else if ident, ok := a.(*ast.Ident); ok && ident.Name == "err" {
				// Again, checking the identifier name is unreliable,
				// but getting the actual type here seems hard/impossible.
				// This means that we'll miss error values with other names.
				args = append(args, argInfo{errorArg, ""})
			} else if call, ok := a.(*ast.CallExpr); ok && isFMTSprintf(call) {
				args = append(args, argInfo{sprintfArg, ""})
			} else {
				args = append(args, argInfo{otherArg, ""})
			}
		}

		addIssue := func(msg, link string) {
			issues = append(issues, &Issue{Pos: fs.Position(x.Pos()), Msg: msg, Link: link})
		}

		const (
			baseURL       = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md"
			printfURL     = baseURL + "#log-vs-logf"
			errPkgURL     = baseURL + "#error-pkg"
			errFmtURL     = baseURL + "#error-fmt"
			logFmtURL     = baseURL + "#log-fmt"
			commonFmtURL  = baseURL + "#common-fmt"
			formattingURL = baseURL + "#Formatting"
			fmtURL        = "https://golang.org/pkg/fmt/#hdr-Printing"
		)

		// Used Logf("Some message") instead of Log("Some message").
		if isFmt && len(args) == 1 {
			addIssue(fmt.Sprintf(`Use %v(%v"<msg>") instead of %v(%v"<msg>")`,
				fmtMap[callName], argPrefix, callName, argPrefix), printfURL)
		}

		// Used Log(fmt.Sprintf(...)) instead of Logf(...)
		if !isFmt && len(args) == 1 && args[0].typ == sprintfArg {
			addIssue(fmt.Sprintf(`Use %v(%v...) instead of %v(%vfmt.Sprintf(...))`,
				fmtMapRev[callName], argPrefix, callName, argPrefix), printfURL)
		}

		// Used Logf("Got %v", i) instead of Log("Got ", i).
		if !isErr && isFmt && len(args) == 2 && args[0].typ == stringArg && strings.HasSuffix(args[0].val, " %v") {
			addIssue(fmt.Sprintf(`Use %v(%v"<msg> ", val) instead of %v(%v"<msg> %%v", val)`,
				fmtMap[callName], argPrefix, callName, argPrefix), printfURL)
		}

		// Used Log("Some error", err) instead of Log("Some error: ", err).
		if !isFmt && !isErr && len(args) == 2 && args[0].typ == stringArg &&
			args[1].typ == errorArg && !strings.HasSuffix(args[0].val, ": ") {
			addIssue(fmt.Sprintf(`%v string arg should end with ": " when followed by error`, callName), logFmtURL)
		}

		// Used errors.Errorf("something failed: %v", err) instead of errors.Wrap(err, "something failed").
		if callName == "errors.Errorf" && len(args) >= 2 && args[0].typ == stringArg &&
			args[len(args)-1].typ == errorArg && strings.HasSuffix(args[0].val, "%v") {
			if len(args) == 2 {
				addIssue(`Use errors.Wrap(err, "<msg>") instead of errors.Errorf("<msg>: %v", err)`, errPkgURL)
			} else {
				addIssue(`Use errors.Wrapf(err, "<msg>", ...) instead of errors.Errorf("<msg>: %v", ..., err)`, errPkgURL)
			}
		}

		// Used Log(err) instead of Log("Some error: ", err).
		if !isErr && len(args) == 1 && args[0].typ == errorArg {
			addIssue(fmt.Sprintf(`Use %v(%v"Something failed: ", err) instead of %v(%verr)`,
				callName, argPrefix, callName, argPrefix), commonFmtURL)
		}

		// Lower-level string checks.
		if len(args) >= 1 && args[0].typ == stringArg && args[0].val != "" {
			str := args[0].val

			// Used Log("Some message.") or Log("Some message!") instead of Log("Some message").
			if strings.LastIndexAny(str, ".!") == len(str)-1 {
				u := logFmtURL
				if isErr {
					u = errFmtURL
				}
				addIssue(fmt.Sprintf("%v string arg should not contain trailing punctuation", callName), u)
			}
			// Used Log("Some message\nMore text") instead of Log("Some message") and Log("More text").
			if strings.Contains(str, "\n") {
				addIssue(fmt.Sprintf("%v string arg should not contain embedded newlines", callName), commonFmtURL)
			}
			// Used Logf("'%s'", ...) instead of Logf("%q", ...).
			if isFmt && (strings.Contains(str, `"%s"`) || strings.Contains(str, `'%s'`) ||
				strings.Contains(str, `"%v"`) || strings.Contains(str, `'%v'`)) {
				addIssue("Use %q to quote values instead of manually quoting them", commonFmtURL)
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
				"expected",
				"failed",
				"found",
				"got",
				"invalid",
				"no",
				"too",
				"unexpected",
				"unknown",
			}
			for _, word := range words {
				if !strings.HasPrefix(strings.ToLower(str), word+" ") {
					continue
				}
				exp := word
				if !isErr {
					exp = strings.ToUpper(word[:1]) + word[1:]
				}
				if strings.HasPrefix(str, exp) {
					continue
				}
				if isErr {
					addIssue("Messages of the error type should not be capitalized", formattingURL)
				} else if isLog {
					addIssue("Log messages should be capitalized", formattingURL)
				} else {
					addIssue("Test failure messages should be capitalized", formattingURL)
				}
			}
		}

		// The number of verbs and the number of arguments must match.
		if isFmt && len(args) >= 1 && args[0].typ == stringArg && countVerbs(args[0].val) != len(args)-1 {
			addIssue("The number of verbs in format literal mismatches with the number of arguments", fmtURL)
		}
		// Used verbs in non *f families.
		if !isFmt && len(args) >= 1 && args[0].typ == stringArg && hasVerbs(args[0].val) {
			addIssue(fmt.Sprintf("%s has verbs in the first string (do you mean %s?)", callName, fmtMapRev[callName]), formattingURL)
		}

		// Error messages should contain some surrounding context.
		if !isLog && len(args) >= 1 && args[0].typ == stringArg && args[0].val == "" {
			addIssue("Error message should have some surrounding context, so must not empty", errFmtURL)
		}
	})

	ast.Walk(v, f)
	return issues
}
