// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenBundleImports(t *testing.T) {
	for _, tc := range []struct {
		code, path string
		expects    []string
	}{
		{
			`package main

import (
	"chromiumos/tast/local/bar/baz"
	"chromiumos/tast/local/bundles/cros/bar/baz"
	"chromiumos/tast/local/bundles/cros/foo/baz"
	"chromiumos/tast/remote/bundles/cros/foo/baz"
)
`,
			"src/chromiumos/tast/local/bundles/cros/foo/testfile.go",
			[]string{
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:5:2: import of chromiumos/tast/local/bundles/cros/bar/baz is only allowed from chromiumos/tast/local/bundles/cros/bar or its descendant or its parent",
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:7:2: import of chromiumos/tast/remote/bundles/cros/foo/baz is only allowed from chromiumos/tast/remote/bundles/cros/foo or its descendant or its parent",
			},
		},
		{
			`package main

import (
	"chromiumos/tast/local/bundles/cros/bar"
	"chromiumos/tast/remote/bundles/cros/bar"
	"chromiumos/tast/remote/bundles/cros/bar/baz"
)
`,
			"src/chromiumos/tast/remote/bundles/cros/testfile.go",
			[]string{
				"src/chromiumos/tast/remote/bundles/cros/testfile.go:4:2: import of chromiumos/tast/local/bundles/cros/bar is only allowed from chromiumos/tast/local/bundles/cros/bar or its descendant or its parent",
			},
		},
		{
			`package main

import (
	"chromiumos/tast/local/bundles/cros/foo"
)
`,
			"src/chromiumos/tast/local/foo/testfile.go",
			[]string{
				"src/chromiumos/tast/local/foo/testfile.go:4:2: import of chromiumos/tast/local/bundles/cros/foo is only allowed from chromiumos/tast/local/bundles/cros/foo or its descendant or its parent",
			},
		},
	} {
		f, fs := parse(tc.code, tc.path)
		issues := ForbiddenBundleImports(fs, f)
		verifyIssues(t, issues, tc.expects)
	}
}
