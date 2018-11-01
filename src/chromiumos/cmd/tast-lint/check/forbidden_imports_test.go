// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenImports(t *testing.T) {
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
