// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestInformational(t *testing.T) {
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
	verifyIssues(t, issues, nil)
}

func TestInformationalDisabled(t *testing.T) {
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
	verifyIssues(t, issues, nil)
}

func TestInformationalCrosbolt(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Contacts: []string{"tast-owners@google.com"},
		Attr:     []string{"group:crosbolt"},
	})
}
`
	const path = "/src/chromiumos/tast/local/crosbolt.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInformationalMainline(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Contacts: []string{"tast-owners@google.com"},
		Attr:     []string{"disabled", "group:mainline"},
		Pre:      chrome.LoggedIn(),
	})
}
`
	const path = "/src/chromiumos/tast/local/mainline.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInformationalParams1(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{"group:mainline"},
		Params: []testing.Param{{
			Name: "param1",
			ExtraAttr: []string{"informational"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"disabled"},
		}, {
			Name: "param3",
			ExtraAttr: []string{},
		}},
	})
}
`
	const path = "/src/chromiumos/tast/local/parameterized1.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	expects := []string{
		path + ":13:4: Newly added tests should be marked as 'informational'.",
	}
	verifyIssues(t, issues, expects)
}

func TestInformationalParams2(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Params: []testing.Param{{
			Name: "param1",
			ExtraAttr: []string{"group:mainline"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"disabled"},
		}, {
			Name: "param3",
			ExtraAttr: []string{"group:crosbolt"},
		}, {
			Name: "param4",
			ExtraAttr: []string{"informational"},
		}},
	})
}
`
	const path = "/src/chromiumos/tast/local/parameterized2.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	expects := []string{
		path + ":6:4: Newly added tests should be marked as 'informational'.",
	}
	verifyIssues(t, issues, expects)
}

func TestInformationalParams3(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{"informational"},
		Params: []testing.Param{{
			Name: "param1",
			ExtraAttr: []string{"group:mainline"},
		}, {
			Name: "param2",
		}, {
			Name: "param3",
			ExtraAttr: []string{"group:crosbolt"},
		}},
	})
}
`
	const path = "/src/chromiumos/tast/local/parameterized3.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}
