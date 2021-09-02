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
	"regexp"
	"strconv"
	"strings"
)

var bundleCategoryRegex = regexp.MustCompile(`^(chromiumos/tast/(?:local|remote)/bundles/\w+)/(\w+)`)

// parseBundlePackage analyzes full package path and returns matching bundle prefix and the category name.
// Returns ok=false if it's not a bundle (sub)package.
//
// Example:
//   "chromiumos/tast/local/bundles/cros/foo"         -> "chromiumos/tast/local/bundles/cros", "foo", true
//   "chromiumos/tast/remote/bundles/crosint/foo/bar" -> "chromiumos/tast/remote/bundles/crosint", "foo", true
//   "chromiumos/tast/local/foo"                      -> "", "", false
func parseBundlePackage(p string) (bundlePkg, category string, ok bool) {
	m := bundleCategoryRegex.FindStringSubmatch(p)
	if len(m) == 0 {
		return "", "", false
	}
	return m[1], m[2], true
}

// ForbiddenBundleImports makes sure libraries in another package under bundles/ are not imported.
func ForbiddenBundleImports(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	mypkg := strings.TrimPrefix(filepath.Dir(filename), "src/")
	myBundlePkg, myCategory, isBundle := parseBundlePackage(mypkg)
	myBundlePath := path.Dir(myBundlePkg)

	var issues []*Issue
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}

		bundlePkg, category, ok := parseBundlePackage(p)
		if !ok {
			continue
		}

		// Allow import from main.go .
		if mypkg == bundlePkg {
			continue
		}

		// Allow import from package category across bundles.
		bundlePath := path.Dir(bundlePkg)
		if isBundle && myBundlePath == bundlePath && myCategory == category {
			continue
		}

		categoryPkg := path.Join(bundlePath, "*", category)

		issues = append(issues, &Issue{
			Pos:  fs.Position(im.Pos()),
			Msg:  fmt.Sprintf("import of %s is only allowed from %s or its descendant", p, categoryPkg),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Scoping-and-shared-code",
		})
	}

	return issues
}
