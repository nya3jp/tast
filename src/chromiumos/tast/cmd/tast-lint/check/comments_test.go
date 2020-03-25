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

// spaceIsNeeded after the function name.
func space() {}

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

// Don't check exported function.
func Foo() {}

// Don't check exported method.
func (x *recv) Bar() {}
`

	f, fs := parse(code, "test.go")

	issues := Comments(fs, f)
	expects := []string{
		`test.go:6:1: Comment on function bad should be of the form "bad ..."`,
		`test.go:11:1: Comment on function space should be of the form "space ..."`,
		`test.go:18:1: Comment on function multiLineBad should be of the form "multiLineBad ..."`,
		`test.go:27:1: Comment on function notGood should be of the form "notGood ..."`,
	}
	verifyIssues(t, issues, expects)
}
