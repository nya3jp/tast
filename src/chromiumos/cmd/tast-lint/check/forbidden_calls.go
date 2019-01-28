// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// ForbiddenCalls checks if any forbidden functions are called.
func ForbiddenCalls(fs *token.FileSet, f *ast.File) []*Issue {
	isUnitTest := isUnitTestFile(fs.Position(f.Package).Filename)
	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return
		}
		// TODO(nya): Support imports with different aliases.
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return
		}

		call := fmt.Sprintf("%s.%s", x.Name, sel.Sel.Name)
		switch call {
		case "context.Background", "context.TODO":
			if !isUnitTest {
				issues = append(issues, &Issue{
					Pos:  fs.Position(x.Pos()),
					Msg:  call + " ignores test timeout; use test function's ctx arg instead",
					Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
				})
			}
		case "fmt.Errorf":
			issues = append(issues, &Issue{
				Pos:  fs.Position(x.Pos()),
				Msg:  "chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
			})
		case "time.Sleep":
			issues = append(issues, &Issue{
				Pos:  fs.Position(x.Pos()),
				Msg:  "time.Sleep ignores context deadline; use time.After instead",
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
			})
		}
	})

	ast.Walk(v, f)
	return issues
}
