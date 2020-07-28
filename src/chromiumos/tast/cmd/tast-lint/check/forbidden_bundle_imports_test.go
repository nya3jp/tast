// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestParseBundlePackage(t *testing.T) {
	type resp struct {
		bundlePkg string
		category  string
		ok        bool
	}
	for _, tc := range []struct {
		p    string
		want resp
	}{
		{
			"chromiumos/tast/local/bundles/cros/foo",
			resp{"chromiumos/tast/local/bundles/cros", "foo", true},
		},
		{
			"chromiumos/tast/remote/bundles/crosint/foo/bar",
			resp{"chromiumos/tast/remote/bundles/crosint", "foo", true},
		},
		{
			"chromiumos/tast/local/foo",
			resp{"", "", false},
		},
		{
			"",
			resp{"", "", false},
		},
		{
			"chromiumos/tast/local/bundles/cros",
			resp{"", "", false},
		},
	} {
		bundlePkg, category, ok := parseBundlePackage(tc.p)
		if got, want := (resp{bundlePkg, category, ok}), tc.want; got != want {
			t.Errorf("parseBundlePackage(%q) returns %+v; want %+v", tc.p, got, want)
		}
	}
}

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
	"chromiumos/tast/local/bundles/crosint/bar"
	"chromiumos/tast/local/bundles/crosint/bar/baz"
	"chromiumos/tast/local/bundles/crosint/foo"
	"chromiumos/tast/local/bundles/crosint/foo/baz"
	"chromiumos/tast/remote/bundles/cros/foo/baz"
)
`,
			"src/chromiumos/tast/local/bundles/cros/foo/testfile.go",
			[]string{
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:5:2: import of chromiumos/tast/local/bundles/cros/bar/baz is only allowed from chromiumos/tast/local/bundles/*/bar or its descendant",
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:7:2: import of chromiumos/tast/local/bundles/crosint/bar is only allowed from chromiumos/tast/local/bundles/*/bar or its descendant",
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:8:2: import of chromiumos/tast/local/bundles/crosint/bar/baz is only allowed from chromiumos/tast/local/bundles/*/bar or its descendant",
				"src/chromiumos/tast/local/bundles/cros/foo/testfile.go:11:2: import of chromiumos/tast/remote/bundles/cros/foo/baz is only allowed from chromiumos/tast/remote/bundles/*/foo or its descendant",
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
				"src/chromiumos/tast/remote/bundles/cros/testfile.go:4:2: import of chromiumos/tast/local/bundles/cros/bar is only allowed from chromiumos/tast/local/bundles/*/bar or its descendant",
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
				"src/chromiumos/tast/local/foo/testfile.go:4:2: import of chromiumos/tast/local/bundles/cros/foo is only allowed from chromiumos/tast/local/bundles/*/foo or its descendant",
			},
		},
	} {
		f, fs := parse(tc.code, tc.path)
		issues := ForbiddenBundleImports(fs, f)
		verifyIssues(t, issues, tc.expects)
	}
}
