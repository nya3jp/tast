// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var bundlePrefix = []string{
	"chromiumos/tast/local/bundles/cros/",
	"chromiumos/tast/remote/bundles/cros/",
}

// bundlePackage analyzes full package path and returns matching bundle prefix and the package name.
// Returns ok=false if it's not a bundle (sub)package.
func bundlePackage(p string) (prefix string, pkg string, ok bool) {
	for _, pref := range bundlePrefix {
		if strings.HasPrefix(p, pref) {
			t := strings.SplitN(strings.TrimPrefix(p, pref), "/", 2)
			return strings.TrimRight(pref, "/"), t[0], true
		}
	}
	return "", "", false
}

// ForbiddenBundleImports makes sure libraries in another package under bundles/ are not imported.
func ForbiddenBundleImports(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	mypkg := strings.TrimPrefix(filepath.Dir(filename), "src/")

	var issues []*Issue
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}

		bundle, pkg, ok := bundlePackage(p)
		if !ok {
			continue
		}

		// Allow import from main.go .
		if mypkg == bundle {
			continue
		}

		expPrefix := path.Join(bundle, pkg)
		if strings.HasPrefix(mypkg+"/", expPrefix+"/") {
			continue
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(im.Pos()),
			Msg:  fmt.Sprintf("import of %s is only allowed from %s or its descendant or its parent", p, expPrefix),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Scoping-and-shared-code",
		})
	}

	return issues
}
