// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import "testing"

func TestComments(t *testing.T) {
	const code = `package pkg

// foo does something.
func foo() {}

// Do something bad.
func bad() {}

func noCommentIsOK() {}

// multiLine comment is OK.
// Yeah.
func multiLine() {}

// linting
// multiLineBad should fail.
func multiLineBad() {}

// bar does something.
func (x *recv) bar() {}

func (x *recv) noCommentIsFine() {}

// This is not good.
func (x *recv) notGood() {}
`

	f, fs := parse(code, "test.go")

	issues := Comments(fs, f)
	expects := []string{
		"test.go:6:1: Function comments should begin with the function name",
		"test.go:15:1: Function comments should begin with the function name",
		"test.go:24:1: Function comments should begin with the function name",
	}
	verifyIssues(t, issues, expects)
}
