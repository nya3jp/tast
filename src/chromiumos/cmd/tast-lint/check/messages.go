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
)

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
		_, isFmt := fmtMap[callName]

		isErr := recvName == "errors"

		type argType int
		const (
			stringArg argType = iota
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
			} else {
				args = append(args, argInfo{otherArg, ""})
			}
		}

		addIssue := func(msg, link string) {
			issues = append(issues, &Issue{Pos: fs.Position(x.Pos()), Msg: msg, Link: link})
		}

		const (
			baseURL      = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md"
			printfURL    = baseURL + "#log-vs-logf"
			errPkgURL    = baseURL + "#error-pkg"
			errFmtURL    = baseURL + "#error-fmt"
			logFmtURL    = baseURL + "#log-fmt"
			commonFmtURL = baseURL + "#common-fmt"
		)

		// Used Logf("Some message") instead of Log("Some message").
		if isFmt && len(args) == 1 {
			addIssue(fmt.Sprintf(`Use %v(%v"<msg>") instead of %v(%v"<msg>")`,
				fmtMap[callName], argPrefix, callName, argPrefix), printfURL)
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
			if strings.Index(str, "\n") != -1 {
				addIssue(fmt.Sprintf("%v string arg should not contain embedded newlines", callName), commonFmtURL)
			}
			// We'd ideally also check that log messages are capitalized, but that causes too many false positives
			// due to messages beginning with daemon names, command lines, etc.
			// Similarly, it'd be nice to check that error values aren't capitalized, but that causes false
			// positives due to proper nouns like "Android" or e.g. D-Bus method call names.
		}
	})

	ast.Walk(v, f)
	return issues
}
