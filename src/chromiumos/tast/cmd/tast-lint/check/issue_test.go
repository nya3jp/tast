// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"testing"
)

func TestDropIgnoredIssues(t *testing.T) {
	const (
		pkg  = "pkg" // package name used in function calls
		code = `package main

func main() {
       pkg.EndOfLine() // An end-of-line comment with NOLINT should result in the line being ignored.
       pkg.Inline("foo" /* An inline comment with NOLINT should also work. */)
       pkg.Multiline("foo", // A NOLINT comment in the middle of a function call should work.
               "bar")
       pkg.CommentStartsOnSameLine() /* While it's not necessarily required behavior, we use the start
                                                                        of the comment group, so this call should be ignored even though
                                                                        the NOLINT appears several lines later. */

       // A NOLINT comment on the line before shouldn't do anything.
       pkg.Uncommented() // Nor an end-of-line comment without that string.
       // Nor a NOLINT comment on the line after.

       pkg.CommentOnNextLine("foo",
               "bar") // This function should be reported, since the NOLINT comment starts on a different line.

       pkg.Outer( // This NOLINT call should only result in the outer function call being skipped.
               pkg.Inner())
}`
	)

	// Walk the code and create an issue for each call to a function in pkg.
	f, fs := parse(code, "file.go")
	var issues []*Issue
	ast.Walk(funcVisitor(func(node ast.Node) {
		if call, ok := node.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if x.Name == pkg {
						issues = append(issues, &Issue{Msg: sel.Sel.Name, Pos: fs.Position(call.Pos())})
					}
				}
			}
		}
	}), f)

	// Check that the only the expected issues remain.
	verifyIssues(t, DropIgnoredIssues(issues, fs, f), []string{
		"file.go:13:8: Uncommented",
		"file.go:16:8: CommentOnNextLine",
		"file.go:20:16: Inner",
	})
}
