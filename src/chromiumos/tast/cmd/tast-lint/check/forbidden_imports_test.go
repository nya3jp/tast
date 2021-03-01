// Copyright 2018 The Chromium OS Authors. All rights reserved.
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

	"chromiumos/tast/errors"

	"github.com/pkg/errors"
)
`
	expects := []string{
		"testfile.go:5:2: chromiumos/tast/errors package should be used instead of errors package",
		"testfile.go:9:2: chromiumos/tast/errors package should be used instead of github.com/pkg/errors package",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenImports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestForbiddenImports_LocalRemote(t *testing.T) {
	const code = `package main

import (
	"chromiumos/tast/common/foo"
	"chromiumos/tast/local/foo"
	"chromiumos/tast/remote/foo"
)
`
	expects := []string{
		"src/chromiumos/tast/local/testfile.go:6:2: Local package should not import remote package chromiumos/tast/remote/foo",
	}

	f, fs := parse(code, "src/chromiumos/tast/local/testfile.go")
	issues := ForbiddenImports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestForbiddenImports_RemoteLocal(t *testing.T) {
	const code = `package main

import (
	"chromiumos/tast/common/foo"
	"chromiumos/tast/local/foo"
	"chromiumos/tast/remote/foo"
)
`
	expects := []string{
		"src/chromiumos/tast/remote/testfile.go:5:2: Remote package should not import local package chromiumos/tast/local/foo",
	}

	f, fs := parse(code, "src/chromiumos/tast/remote/testfile.go")
	issues := ForbiddenImports(fs, f)
	verifyIssues(t, issues, expects)
}
