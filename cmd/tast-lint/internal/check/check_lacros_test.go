// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"

	"go.chromium.org/tast/cmd/tast-lint/internal/git"
)

const lacrosPath = "/src/chromiumos/tast/local/bundles/cros/lacros/foo.go"

func TestLacrosSoftwareDepsSuccess(t *testing.T) {
	const code = `package lacros
func init() {
	testing.AddTest(&testing.Test{
		Func:         Foo,
		Desc:         "Tests tast-lint for lacros package",
		Contacts:     []string{"tast-owners@google.com"},
		BugComponent: "b:1034625",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"lacros_stable", "lacros"},
	})
}
`
	f, fs := parse(code, lacrosPath)
	issues := TestDeclarations(fs, f, git.CommitFile{}, false)
	verifyIssues(t, issues, nil)
}

func TestLacrosSoftwareDepsFailure(t *testing.T) {
	const code = `package lacros
func init() {
	testing.AddTest(&testing.Test{
		Func:         Foo,
		Desc:         "Tests tast-lint for lacros package",
		Contacts:     []string{"tast-owners@google.com"},
		BugComponent: "b:1034625",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"lacros_stable"},
	})
}
`
	f, fs := parse(code, lacrosPath)
	issues := TestDeclarations(fs, f, git.CommitFile{}, false)
	expects := []string{
		lacrosPath + ":9:3: Test SoftwareDeps dep:lacros should be added along with dep:lacros_{un}stable",
	}
	verifyIssues(t, issues, expects)
}

func TestLacrosSoftwareDepsInParamsSuccess(t *testing.T) {
	const code = `package lacros
func init() {
	testing.AddTest(&testing.Test{
		Func:         Foo,
		Desc:         "Tests tast-lint for lacros package",
		Contacts:     []string{"tast-owners@google.com"},
		BugComponent: "b:1034625",
		LacrosStatus: testing.LacrosVariantExists,
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Params: []testing.Param{{
			Name:      "base",
		}, {
			Name:      "stable",
			ExtraSoftwareDeps: []string{"lacros_stable"},
		}, {
			Name:      "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
		}},
	})
}
`
	f, fs := parse(code, lacrosPath)
	issues := TestDeclarations(fs, f, git.CommitFile{}, false)
	verifyIssues(t, issues, nil)
}

func TestLacrosSoftwareDepsInParamsFailure(t *testing.T) {
	const code = `package lacros
func init() {
	testing.AddTest(&testing.Test{
		Func:         Foo,
		Desc:         "Tests tast-lint for lacros package",
		Contacts:     []string{"tast-owners@google.com"},
		BugComponent: "b:1034625",
		LacrosStatus: testing.LacrosVariantExists,
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},	// "lacros" is omitted purposely to raise a lint issue.
		Params: []testing.Param{{
			Name:      "stable",
			ExtraSoftwareDeps: []string{"lacros_stable"},
		}, {
			Name:      "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
		}},
	})
}
`
	f, fs := parse(code, lacrosPath)
	issues := TestDeclarations(fs, f, git.CommitFile{}, false)
	expects := []string{
		lacrosPath + ":10:3: Test SoftwareDeps dep:lacros should be added along with dep:lacros_{un}stable",
	}
	verifyIssues(t, issues, expects)
}
