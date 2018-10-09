// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestErrorsImports(t *testing.T) {
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

	f, fs := parse(code)
	issues := ErrorsImports(fs, f)
	verifyIssues(t, issues, expects)
}

func TestFmtPrintf(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
}
`
	expects := []string{
		"testfile.go:11:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
	}

	f, fs := parse(code)
	issues := FmtErrorf(fs, f)
	verifyIssues(t, issues, expects)
}
