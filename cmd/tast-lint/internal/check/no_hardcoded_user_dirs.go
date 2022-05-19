// Copyright 2022 The ChromiumOS Authors.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"

	"golang.org/x/tools/go/ast/astutil"
)

// NoHardcodedUserDirs ensures that as we migrate away from the
// /home/chronos/user* bind mount, further additions are not added.
func NoHardcodedUserDirs(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	// A map of exported vars that are either in the process of migrating and
	// further usage is not allowed or they have been migrated and should not be
	// readded.
	disallowedExportedVars := map[string]struct{}{
		"filesapp.DownloadPath": {},
		"filesapp.MyFilesPath":  {},
	}

	const docsLink = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Logged-in-users-home-directory"

	// Match any paths that are parented at /home/chronos/user but exclude any
	// matches that might be part of a comment.
	mountRegex := regexp.MustCompile(`^"/home/chronos/user[^\s]*"$`)

	// Traverse the syntax tree to identify either string literals that reference
	// the user bind mount OR exported package variables that are known or have
	// been known to reference the bind mount.
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		switch value := c.Node().(type) {
		case *ast.SelectorExpr:
			switch packageName := value.X.(type) {
			case *ast.Ident:
				exportedVar := fmt.Sprintf("%s.%s", packageName.String(), value.Sel.String())
				if _, ok := disallowedExportedVars[exportedVar]; ok {
					issues = append(issues, &Issue{
						Pos:  fs.Position(packageName.Pos()),
						Msg:  exportedVar + " references the /home/chronos/user bind mount which is being deprecated, please use the cryptohome package instead",
						Link: docsLink,
					})
				}
			}
			break
		case *ast.BasicLit:
			if mountRegex.MatchString(value.Value) {
				issues = append(issues, &Issue{
					Pos:  fs.Position(value.Pos()),
					Msg:  "A reference to the /home/chronos/user bind mount was found which is being deprecated, please use the cryptohome package instead",
					Link: docsLink,
				})
			}
			break
		}
		return true
	}, nil)

	return issues
}
