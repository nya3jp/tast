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
	"chromiumos/tast/common/foo"
	"chromiumos/tast/local/foo"
	"chromiumos/tast/remote/foo"
	"go.chromium.org/tast/core/tastuseonly/run"
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
			"src/chromiumos/tast/local/testfile.go:7:2: Test package should not import tastuseonly package go.chromium.org/tast/core/tastuseonly/run",
		},
	}, {
		filepath: "src/chromiumos/tast/remote/testfile.go",
		want: []string{
			"src/chromiumos/tast/remote/testfile.go:5:2: Non-local package should not import local package chromiumos/tast/local/foo",
			"src/chromiumos/tast/remote/testfile.go:7:2: Test package should not import tastuseonly package go.chromium.org/tast/core/tastuseonly/run",
		},
	}, {
		filepath: "src/chromiumos/tast/common/testfile.go",
		want: []string{
			"src/chromiumos/tast/common/testfile.go:5:2: Non-local package should not import local package chromiumos/tast/local/foo",
			"src/chromiumos/tast/common/testfile.go:6:2: Non-remote package should not import remote package chromiumos/tast/remote/foo",
		},
	}, {
		filepath: "src/chromiumos/tast/common/testfile_test.go",
		want:     nil,
	}} {
		f, fs := parse(code, tc.filepath)
		issues := ForbiddenImports(fs, f)
		verifyIssues(t, issues, tc.want)
	}
}

// TestForbiddenImports_MovedPackages makes sure packages to be removed after move
// is completed. will not be called again.
// TODO: b/187792551 -- remove to check after chromiumos/tast/dut os removed.
func TestForbiddenImports_MovedPackages(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"chromiumos/tast/dut"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
)
`
	expects := []string{
		"testfile.go:6:2: go.chromium.org/tast/core/dut package should be used instead of chromiumos/tast/dut package",
		"testfile.go:7:2: go.chromium.org/tast/core/ctxutil package should be used instead of chromiumos/tast/ctxutil package",
		"testfile.go:8:2: go.chromium.org/tast/core/errors package should be used instead of chromiumos/tast/errors package",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenImports(fs, f)
	verifyIssues(t, issues, expects)
}
