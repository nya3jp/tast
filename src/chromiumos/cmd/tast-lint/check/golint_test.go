// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestGolint(t *testing.T) {
	const code = `package main

import "chromiumos/tast/errors"

func Kitten() {}
`
	expects := []string{
		"testfile.go:5:1: exported function Kitten should have comment or be unexported",
	}

	issues := Golint("testfile.go", []byte(code), false)
	verifyIssues(t, issues, expects)
}

func TestGolint_NoLint(t *testing.T) {
	const code = `package main

import "chromiumos/tast/errors"

func Kitten() {} // NOLINT
`
	issues := Golint("testfile.go", []byte(code), false)
	verifyIssues(t, issues, nil)
}

func TestGolint_TestMainFunction(t *testing.T) {
	const code = `package main

import "chromiumos/tast/errors"

func Kitten() {}
`
	issues := Golint("src/chromiumos/tast/local/bundles/cros/example/kitten.go", []byte(code), false)
	// Test main functions can be exported without comments.
	verifyIssues(t, issues, nil)
}
