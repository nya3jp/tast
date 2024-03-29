// Copyright 2019 The ChromiumOS Authors
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
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/keyboard.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInformationalDisabled(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/pass.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInformationalDisabledNil(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: nil,
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/pass.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}

func TestInformationalDisabledEmpty(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{},
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/pass.go"
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
	const path = "/src/go.chromium.org/tast-tests/cros/local/crosbolt.go"
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
		}, {
			Name: "param3",
			ExtraAttr: nil,
		}, {
			Name: "param4",
			ExtraAttr: []string{},
		}},
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/parameterized1.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	expects := []string{
		path + ":8:6: Newly added tests should be marked as 'informational'.",
		path + ":12:4: Newly added tests should be marked as 'informational'.",
		path + ":15:4: Newly added tests should be marked as 'informational'.",
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
			ExtraAttr: []string{"group:crosbolt"},
		}, {
			Name: "param3",
			ExtraAttr: []string{"group:mainline", "informational"},
		}},
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/parameterized2.go"
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
			ExtraAttr: []string{"group:mainline"},
		}},
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/parameterized3.go"
	f, fs := parse(code, path)
	issues := VerifyInformationalAttr(fs, f)
	verifyIssues(t, issues, nil)
}
