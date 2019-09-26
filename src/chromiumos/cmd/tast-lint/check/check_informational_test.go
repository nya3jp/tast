// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestInformationalAttr(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Func:     Fail,
		Desc:     "Always fails",
		Contacts: []string{"tast-owners@google.com"},
		Attr:     []string{"disabled"},
	})
}
`
	const path = "/src/chromiumos/tast/local/bundles/cros/example/fail.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	expects := []string{
		path + ":7:3: Newly added tests should be marked as 'informational'.",
	}
	verifyIssues(t, issues, expects)
}

// TestInformationalAttrOK checks that the test surely have 'informational' attibutes
// doesn't occur the error
func TestInformationalAttrOK(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Func:         Keyboard,
		Desc:         "Demonstrates injecting keyboard events",
		Contacts:     []string{"tast-owners@google.com"},
		Attr:         []string{"informational"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}
`
	const path = "/src/chromiumos/tast/local/bundles/cros/example/keyboard.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	expects := []string{}
	verifyIssues(t, issues, expects)
}
