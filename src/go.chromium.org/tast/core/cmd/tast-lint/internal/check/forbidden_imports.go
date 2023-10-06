// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

var localPkgRE = regexp.MustCompile(`go.chromium.org/tast-.*/local/.*$`)
var remotePkgRE = regexp.MustCompile(`go.chromium.org/tast-.*/remote/.*$`)
var localPkgPrefixRE = regexp.MustCompile(`^go.chromium.org/tast-.*/local/.*$`)
var remotePkgPrefixRE = regexp.MustCompile(`^go.chromium.org/tast-.*/remote/.*$`)

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
				Msg:  fmt.Sprintf("go.chromium.org/tast/core/errors package should be used instead of %s package", p),
				Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Error-construction",
			})
		}
	}

	// local <-> remote, common -> {local, remote} dependencies are forbidden.
	path := fs.File(f.Pos()).Name()
	if !isUnitTestFile(path) {
		const (
			publicPkg          = "go.chromium.org/tast-tests/"
			publicLocalBundle  = "go.chromium.org/tast-tests/cros/local/bundles"
			publicRemoteBundle = "go.chromium.org/tast-tests/cros/remote/bundles"
		)
		localFile := localPkgRE.MatchString(path)
		remoteFile := remotePkgRE.MatchString(path)
		privateFile := !strings.Contains(path, publicPkg)
		for _, im := range f.Imports {
			p, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				continue
			}
			importLocal := localPkgPrefixRE.MatchString(p)
			importRemote := remotePkgPrefixRE.MatchString(p)
			importPublicBundles := strings.HasPrefix(p, publicLocalBundle) || strings.HasPrefix(p, publicRemoteBundle)

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
			if privateFile && importPublicBundles {
				issues = append(issues, &Issue{
					Pos:  fs.Position(im.Pos()),
					Msg:  fmt.Sprintf("Private repository should not import Non-private bundle %v", p),
					Link: link,
				})
			}
		}
	}

	return issues
}
