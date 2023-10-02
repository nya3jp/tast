// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenImports_ErrorPackage(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"errors"

	"go.chromium.org/tast/core/errors"

	"github.com/pkg/errors"
)
`
	expects := []string{
		"testfile.go:5:2: go.chromium.org/tast/core/errors package should be used instead of errors package",
		"testfile.go:9:2: go.chromium.org/tast/core/errors package should be used instead of github.com/pkg/errors package",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenImports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestForbiddenImports(t *testing.T) {
	const code = `package main

import (
	"go.chromium.org/tast-tests/cros/common/foo"
	"go.chromium.org/tast-tests/cros/local/foo"
	"go.chromium.org/tast-tests/cros/remote/foo"
	"go.chromium.org/tast-tests/cros/local/bundles/foo"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/foo"
	"some/other/package"
)
`

	for _, tc := range []struct {
		filepath string
		want     []string
	}{{
		filepath: "src/go.chromium.org/tast-tests/cros/local/testfile.go",
		want: []string{
			"src/go.chromium.org/tast-tests/cros/local/testfile.go:6:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/foo",
			"src/go.chromium.org/tast-tests/cros/local/testfile.go:8:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/bundles/foo",
		},
	}, {
		filepath: "src/go.chromium.org/tast-tests/cros/remote/testfile.go",
		want: []string{
			"src/go.chromium.org/tast-tests/cros/remote/testfile.go:5:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/foo",
			"src/go.chromium.org/tast-tests/cros/remote/testfile.go:7:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/bundles/foo",
		},
	}, {
		filepath: "src/go.chromium.org/tast-tests/cros/common/testfile.go",
		want: []string{
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:5:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/foo",
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:6:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/foo",
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:7:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/bundles/foo",
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:8:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/bundles/foo",
		},
	}, {
		filepath: "src/go.chromium.org/tast-tests-private/crosint/common/testfile.go",
		want: []string{
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:5:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/foo",
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:6:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/foo",
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:7:2: Non-local package should not import local package go.chromium.org/tast-tests/cros/local/bundles/foo",
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:7:2: Private repository should not import Non-private bundle go.chromium.org/tast-tests/cros/local/bundles/foo",
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:8:2: Non-remote package should not import remote package go.chromium.org/tast-tests/cros/remote/bundles/foo",
			"src/go.chromium.org/tast-tests-private/crosint/common/testfile.go:8:2: Private repository should not import Non-private bundle go.chromium.org/tast-tests/cros/remote/bundles/foo",
		},
	}, {
		filepath: "src/go.chromium.org/tast-tests/cros/common/testfile_test.go",
		want:     nil,
	}} {
		f, fs := parse(code, tc.filepath)
		issues := ForbiddenImports(fs, f)
		verifyIssues(t, issues, tc.want)
	}
}
