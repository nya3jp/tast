// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// ForbiddenFlashromSubprocess checks if "flashrom" command is called.
func ForbiddenFlashromSubprocess(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	const (
		localPkg  = "chromiumos/tast/local"
		remotePkg = "chromiumos/tast/remote"
	)

	path := fs.File(f.Pos()).Name()
	localTests := strings.Contains(path, localPkg)
	remoteTests := strings.Contains(path, remotePkg)

	// We only checks the changes in local test or remote test.
	if !localTests && !remoteTests {
		return issues
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if callExpr, ok := n.(*ast.CallExpr); ok {
			isForbidden(callExpr, &issues, fs)
		}
		return true
	})

	return issues
}

func isForbidden(expr ast.Expr, issues *[]*Issue, fs *token.FileSet) {
	v, _ := expr.(*ast.CallExpr)
	if selector, ok := v.Fun.(*ast.SelectorExpr); ok {
		for _, rule := range []struct {
			commandName  string
			parameterIdx int // 0 based idx
		}{
			{
				commandName:  "CommandContext",
				parameterIdx: 1,
			},
		} {
			if selector.Sel.Name == rule.commandName {
				if len(v.Args) > rule.parameterIdx {
					arg, ok := v.Args[rule.parameterIdx].(*ast.BasicLit)
					if ok && arg.Kind == token.STRING {
						if arg.Value == `"flashrom"` || arg.Value == `"/usr/sbin/flashrom"` {
							*issues = append(*issues, &Issue{
								Pos:  fs.Position(v.Pos()),
								Msg:  "Please don't use flashrom subprocess but use flashrom_library instead.",
								Link: "https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/common/flashrom/",
							})
						}
					}

				}
			}
		}
	}
}
