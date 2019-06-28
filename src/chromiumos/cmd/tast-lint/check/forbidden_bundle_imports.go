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

// bundlePackage analyzes full package path and returns matching bundle prefix and the category name.
// Returns ok=false if it's not a bundle (sub)package.
//
// Example:
//   "chromiumos/tast/local/bundles/cros/foo"      -> "chromiumos/tast/local/bundles/cros", "foo", true
//   "chromiumos/tast/remote/bundles/cros/foo/bar" -> "chromiumos/tast/remote/bundles/cros", "foo", true
//   "chromiumos/tast/local/foo"                   -> "", "", false
func parseBundlePackage(p string) (bundlePkg string, category string, ok bool) {
	t := bundleCategoryRegex.FindStringSubmatch(p)
	if len(t) == 0 {
		return "", "", false
	}
	return t[1], t[2], true
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

		bundle, category, ok := parseBundlePackage(p)
		if !ok {
			continue
		}

		// Allow import from main.go .
		if mypkg == bundle {
			continue
		}

		categoryPkg := path.Join(bundle, category)
		if strings.HasPrefix(mypkg+"/", categoryPkg+"/") {
			continue
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(im.Pos()),
			Msg:  fmt.Sprintf("import of %s is only allowed from %s or its descendant", p, categoryPkg),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Scoping-and-shared-code",
		})
	}

	return issues
}
