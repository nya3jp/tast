// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestGolint(t *testing.T) {
	const code = `package main

func Kitten() {}
`
	expects := []string{
		"testfile.go:3:1: exported function Kitten should have comment or be unexported",
	}

	issues := Golint("testfile.go", []byte(code), false)
	verifyIssues(t, issues, expects)
}

func TestGolint_UnexportedTypeInAPI(t *testing.T) {
	const code = `package main

type cute bool

// Kitten is cute.
func Kitten() cute { return true }
`
	issues := Golint("testfile.go", []byte(code), false)
	verifyIssues(t, issues, nil)
}

func TestGolint_TestMainFunction(t *testing.T) {
	const code = `package main

func Kitten() {}
`
	issues := Golint("src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/kitten.go", []byte(code), false)
	// Test main functions can be exported without comments.
	verifyIssues(t, issues, nil)
}

func TestGolint_AcceptId(t *testing.T) {
	const code = `package main

// testId is acceptable.
func testId(tId string) string {
	return tId
}

// methodId is acceptatble
func (m *structType) methodId(tId string) string {
	return m.returnId + "_" + tId
}
`
	issues := Golint("testfile.go", []byte(code), false)
	verifyIssues(t, issues, nil)
}
