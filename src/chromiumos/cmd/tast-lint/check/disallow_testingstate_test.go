// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestTestingState(t *testing.T) {
	const path1 = "/src/chromiumos/tast/local/test1.go"
	const path2 = "/src/chromiumos/tast/local/arc/pre.go"
	const code = `package main
  func A(id int.abc, a *float64.hoge, b *testing.State) {}
  func B(dame *testing.State) {
		fn(decl, func(a int, param *testing.State) bool {
			return false
		})
	}
`
	f, fs := parse(code, path1)
	issues := VerifyTestingState(fs, f)
	expects := []string{
		path1 + ":2:42: 'testing.State' should not be used in support packages",
		path1 + ":3:16: 'testing.State' should not be used in support packages",
		path1 + ":4:31: 'testing.State' should not be used in support packages",
	}
	verifyIssues(t, issues, expects)

	// Test for specific allowed files
	f2, fs2 := parse(code, path2)
	issues2 := VerifyTestingState(fs2, f2)
	expects2 := []string{}
	verifyIssues(t, issues2, expects2)
}
