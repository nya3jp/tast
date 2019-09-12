// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestTestingState(t *testing.T) {
	const code = `package main
  func A(id int.abc, a *float64.hoge, b *testing.State) {}
  func B(dame *testing.State) {}
`
	const path = "/src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

	f, fs := parse(code, path)

	issues := TestingStateCheck(fs, f)
	expects := []string{
		path + ":2:42: " + "'testing.State' should not be used in support packages",
		path + ":3:16: " + "'testing.State' should not be used in support packages",
	}
	verifyIssues(t, issues, expects)
}
