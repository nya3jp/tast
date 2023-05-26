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
	"chromiumos/tast/local/foo"
	"chromiumos/tast/remote/foo"
	"some/other/package"
)
`

	for _, tc := range []struct {
		filepath string
		want     []string
	}{{
		filepath: "src/chromiumos/tast/local/testfile.go",
		want: []string{
			"src/chromiumos/tast/local/testfile.go:6:2: Non-remote package should not import remote package chromiumos/tast/remote/foo",
		},
	}, {
		filepath: "src/chromiumos/tast/remote/testfile.go",
		want: []string{
			"src/chromiumos/tast/remote/testfile.go:5:2: Non-local package should not import local package chromiumos/tast/local/foo",
		},
	}, {
		filepath: "src/go.chromium.org/tast-tests/cros/common/testfile.go",
		want: []string{
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:5:2: Non-local package should not import local package chromiumos/tast/local/foo",
			"src/go.chromium.org/tast-tests/cros/common/testfile.go:6:2: Non-remote package should not import remote package chromiumos/tast/remote/foo",
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
