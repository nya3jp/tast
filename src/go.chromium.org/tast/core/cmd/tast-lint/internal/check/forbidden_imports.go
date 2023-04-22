// Copyright 2018 The ChromiumOS Authors
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
		// TODO: b/187792551 -- remove to check after chromiumos/tast in tast repo is removed.
		if p == "chromiumos/tast/dut" {
			issues = append(issues, &Issue{
				Pos: fs.Position(im.Pos()),
				Msg: fmt.Sprintf("go.chromium.org/tast/core/dut package should be used instead of %s package", p),
			})
		}
		if p == "chromiumos/tast/ctxutil" {
			issues = append(issues, &Issue{
				Pos: fs.Position(im.Pos()),
				Msg: fmt.Sprintf("go.chromium.org/tast/core/ctxutil package should be used instead of %s package", p),
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
			// TODO: b/187792551 remove this check after tastuseonly is renamed back to internal.
			if strings.Contains(p, "tastuseonly") {
				forbidden := localFile || remoteFile
				if !forbidden {
					testPackagePaths := []string{
						"chromiumos/tast/common",
						"chromiumos/tast/restrictions",
						"chromiumos/tast/services",
					}
					for _, t := range testPackagePaths {
						if strings.HasPrefix(p, t) {
							forbidden = true
							break
						}
					}
				}
				if forbidden {
					issues = append(issues, &Issue{
						Pos:  fs.Position(im.Pos()),
						Msg:  fmt.Sprintf("Test package should not import tastuseonly package %v", p),
						Link: link,
					})
				}
			}
		}
	}

	return issues
}
