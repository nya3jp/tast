// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// ForbiddenBundleImports makes sure libraries in another package under bundles/ are not imported.
func ForbiddenBundleImports(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	mypkg := strings.TrimPrefix(filepath.Dir(filename), "src/")

	bundlePrefix := []string{
		"chromiumos/tast/local/bundles/cros/",
		"chromiumos/tast/remote/bundles/cros/",
	}

	analyze := func(fullpkg string) (prefix string, pkg string, subpkg string) {
		for _, pref := range bundlePrefix {
			if !strings.HasPrefix(fullpkg, pref) {
				continue
			}
			rel := strings.TrimPrefix(fullpkg, pref)
			t := strings.SplitN(rel, "/", 2)
			if len(t) < 2 {
				continue
			}
			return pref, t[0], t[1]
		}
		return "", "", ""
	}

	var issues []*Issue
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}

		pref, pkg, subpkg := analyze(p)
		if subpkg == "" {
			continue
		}

		exppkg := filepath.Join(pref, pkg)
		if mypkg == exppkg {
			continue
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(im.Pos()),
			Msg:  fmt.Sprintf("%s can be imported only from %s, but yours is %s", p, exppkg, mypkg),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#bundle-subpackage",
		})
	}

	return issues
}
