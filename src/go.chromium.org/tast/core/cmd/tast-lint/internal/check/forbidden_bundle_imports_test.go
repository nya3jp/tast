// Copyright 2019 The ChromiumOS Authors
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
			"go.chromium.org/tast-tests/cros/local/bundles/cros/foo",
			resp{"go.chromium.org/tast-tests/cros/local/bundles/cros", "foo", true},
		},
		{
			"go.chromium.org/tast-tests/cros/remote/bundles/crosint/foo/bar",
			resp{"go.chromium.org/tast-tests/cros/remote/bundles/crosint", "foo", true},
		},
		{
			"go.chromium.org/tast-tests/cros/local/foo",
			resp{"", "", false},
		},
		{
			"",
			resp{"", "", false},
		},
		{
			"go.chromium.org/tast-tests/cros/local/bundles/cros",
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
	"go.chromium.org/tast-tests/cros/local/bar/baz"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/bar/baz"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/foo/baz"
	"go.chromium.org/tast-tests/cros/local/bundles/crosint/bar"
	"go.chromium.org/tast-tests/cros/local/bundles/crosint/bar/baz"
	"go.chromium.org/tast-tests/cros/local/bundles/crosint/foo"
	"go.chromium.org/tast-tests/cros/local/bundles/crosint/foo/baz"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/foo/baz"
)
`,
			"src/go.chromium.org/tast-tests/cros/local/bundles/cros/foo/testfile.go",
			[]string{
				"src/go.chromium.org/tast-tests/cros/local/bundles/cros/foo/testfile.go:5:2: import of go.chromium.org/tast-tests/cros/local/bundles/cros/bar/baz is only allowed from go.chromium.org/tast-tests/cros/local/bundles/*/bar or its descendant",
				"src/go.chromium.org/tast-tests/cros/local/bundles/cros/foo/testfile.go:7:2: import of go.chromium.org/tast-tests/cros/local/bundles/crosint/bar is only allowed from go.chromium.org/tast-tests/cros/local/bundles/*/bar or its descendant",
				"src/go.chromium.org/tast-tests/cros/local/bundles/cros/foo/testfile.go:8:2: import of go.chromium.org/tast-tests/cros/local/bundles/crosint/bar/baz is only allowed from go.chromium.org/tast-tests/cros/local/bundles/*/bar or its descendant",
				"src/go.chromium.org/tast-tests/cros/local/bundles/cros/foo/testfile.go:11:2: import of go.chromium.org/tast-tests/cros/remote/bundles/cros/foo/baz is only allowed from go.chromium.org/tast-tests/cros/remote/bundles/*/foo or its descendant",
			},
		},
		{
			`package main

import (
	"go.chromium.org/tast-tests/cros/local/bundles/cros/bar"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bar"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bar/baz"
)
`,
			"src/go.chromium.org/tast-tests/cros/remote/bundles/cros/testfile.go",
			[]string{
				"src/go.chromium.org/tast-tests/cros/remote/bundles/cros/testfile.go:4:2: import of go.chromium.org/tast-tests/cros/local/bundles/cros/bar is only allowed from go.chromium.org/tast-tests/cros/local/bundles/*/bar or its descendant",
			},
		},
		{
			`package main

import (
	"go.chromium.org/tast-tests/cros/local/bundles/cros/foo"
)
`,
			"src/go.chromium.org/tast-tests/cros/local/foo/testfile.go",
			[]string{
				"src/go.chromium.org/tast-tests/cros/local/foo/testfile.go:4:2: import of go.chromium.org/tast-tests/cros/local/bundles/cros/foo is only allowed from go.chromium.org/tast-tests/cros/local/bundles/*/foo or its descendant",
			},
		},
	} {
		f, fs := parse(tc.code, tc.path)
		issues := ForbiddenBundleImports(fs, f)
		verifyIssues(t, issues, tc.expects)
	}
}
