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
func parseBundlePackage(p string) (bundlePkg string, category string, ok bool) {
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

	// Ignore known valid use cases.
	var allowMap = map[string]string{
		"chromiumos/tast/local/bundles/cros/webrtc/camera": "src/chromiumos/tast/local/bundles/cros/camera/getusermedia/get_user_media.go",
	}
	filepath := fs.Position(f.Package).Filename

	var issues []*Issue
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}

		if path, ok := allowMap[p]; ok && strings.HasSuffix(filepath, path) {
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

		categoryPkg := path.Join(bundlePkg, category)
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
