// Copyright 2018 The Chromium OS Authors. All rights reserved.
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
	"chromiumos/tast/local/foo/baz"
	"chromiumos/tast/remote/bundles/cros/foo/baz"
)
`
	expects := []string{
		"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:4:2: chromiumos/tast/local/bundles/cros/bar/baz can be imported only from chromiumos/tast/local/bundles/cros/bar, but yours is chromiumos/tast/local/bundles/cros/foo",
		"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:7:2: chromiumos/tast/remote/bundles/cros/foo/baz can be imported only from chromiumos/tast/remote/bundles/cros/foo, but yours is chromiumos/tast/local/bundles/cros/foo",
	}

	f, fs := parse(code, "src/chromiumos/tast/local/bundles/cros/foo/testfile.go")
	issues := ForbiddenBundleImports(fs, f)
	verifyIssues(t, issues, expects)
}
