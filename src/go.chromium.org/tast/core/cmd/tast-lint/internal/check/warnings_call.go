// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

func ctext(list []*ast.CommentGroup) string {
	var buf bytes.Buffer
	for _, g := range list {
		buf.WriteString(g.Text())
	}
	return buf.String()
}

// WarningCalls checks if any forbidden functions are called.
func WarningCalls(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	var issues []*Issue
	allowedSleepSet := map[string]struct{}{}
	cmap := ast.NewCommentMap(fs, f, f.Comments)
	// Set of GoBigSleepLint sleep calls
	for n, list := range cmap {
		if strings.Contains(ctext(list), "GoBigSleepLint") {
			key := fs.Position(n.Pos()).Filename + strconv.Itoa(fs.Position(n.Pos()).Line)
			allowedSleepSet[key] = struct{}{}
		}
	}
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		sel, ok := c.Node().(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		methodName := sel.Sel.Name
		call := fmt.Sprintf("%s.%s", x.Name, methodName)
		switch call {
		case "testing.Sleep":
			key := fs.Position(x.Pos()).Filename + strconv.Itoa(fs.Position(x.Pos()).Line)
			if _, ok := allowedSleepSet[key]; !ok {
				issues = append(issues, &Issue{
					Pos:     fs.Position(x.Pos()),
					Msg:     "testing.sleep causes flakiness in the test. If sleep is necessary, please add a comment with GoBigSleepLint explaining the justification",
					Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
					Warning: true,
				})
			}
		}
		return true
	}, nil)
	return issues
}
