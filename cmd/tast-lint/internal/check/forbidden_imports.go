// Copyright 2018 The Chromium OS Authors. All rights reserved.
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

// ForbiddenImports makes sure tests don't have forbidden imports.
func ForbiddenImports(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	// errors packages not for Tast are forbidden.
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		if p == "errors" || p == "github.com/pkg/errors" {
			issues = append(issues, &Issue{
				Pos:  fs.Position(im.Pos()),
				Msg:  fmt.Sprintf("chromiumos/tast/errors package should be used instead of %s package", p),
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
			})
		}
	}

	// local <-> remote, common -> {local, remote} dependencies are forbidden.
	path := fs.File(f.Pos()).Name()
	if !isUnitTestFile(path) {
		const (
			localPkg  = "chromiumos/tast/local"
			remotePkg = "chromiumos/tast/remote"
		)
		localFile := strings.Contains(path, localPkg)
		remoteFile := strings.Contains(path, remotePkg)
		for _, im := range f.Imports {
			p, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				continue
			}

			importLocal := strings.HasPrefix(p, localPkg)
			importRemote := strings.HasPrefix(p, remotePkg)

			const link = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Code-location"
			if !localFile && importLocal {
				issues = append(issues, &Issue{
					Pos:  fs.Position(im.Pos()),
					Msg:  fmt.Sprintf("Non-local package should not import local package %v", p),
					Link: link,
				})
			}
			if !remoteFile && importRemote {
				issues = append(issues, &Issue{
					Pos:  fs.Position(im.Pos()),
					Msg:  fmt.Sprintf("Non-remote package should not import remote package %v", p),
					Link: link,
				})
			}
		}
	}

	return issues
}
