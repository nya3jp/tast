// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestPackageCommentNoComment(t *testing.T) {
	const code = `
package newpackage
`
	const path = "newpackage/newpackage.go"
	f, fs := parse(code, path)
	issues := PackageComment(fs, f)
	expects := []string{
		path + ":2:1: document of newly created package 'newpackage' is required",
	}
	verifyIssues(t, issues, expects)
}

func TestPackageCommentBadFormat(t *testing.T) {
	const code = `
// Copyright

// newpackage do nothing
package newpackage
`
	const path = "newpackage/newpackage.go"
	f, fs := parse(code, path)
	issues := PackageComment(fs, f)
	expects := []string{
		path + ":5:1: document of newly created package need to start with '// Package newpackage...'",
	}
	verifyIssues(t, issues, expects)
}

func TestPackageCommentOK(t *testing.T) {
	const code = `
// Copyright

// Package newpackage do nothing
// do nothing
package newpackage
`
	const path = "newpackage/newpackage.go"
	f, fs := parse(code, path)
	issues := PackageComment(fs, f)
	verifyIssues(t, issues, nil)
}
