// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenBundleImports(t *testing.T) {
	const code = `package main

import (
	"chromiumos/tast/local/bundles/cros/bar/baz"
	"chromiumos/tast/local/bundles/cros/foo/baz"
	"chromiumos/tast/local/bar/baz"
	"chromiumos/tast/remote/bundles/cros/foo/baz"
)
`
	expects := []string{
		"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:4:2: import of chromiumos/tast/local/bundles/cros/bar/baz is not allowed from chromiumos/tast/local/bundles/cros/foo; must be from chromiumos/tast/local/bundles/cros/bar or its descendant",
		"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:7:2: import of chromiumos/tast/remote/bundles/cros/foo/baz is not allowed from chromiumos/tast/local/bundles/cros/foo; must be from chromiumos/tast/remote/bundles/cros/foo or its descendant",
	}

	f, fs := parse(code, "src/chromiumos/tast/local/bundles/cros/foo/testfile.go")
	issues := ForbiddenBundleImports(fs, f)
	verifyIssues(t, issues, expects)
}
