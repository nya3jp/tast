// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

var testcode = []string{
	`package main
			import (
				"chromiumos/tast/common/policy"
				"chromiumos/tast/testing"
			)
		func init() {
				testing.AddTest(&testing.Test{
					Func: test,
					SearchFlags: []*testing.StringPair{
						{Key: &policy.AllowDinosaurEasterEgg{}, Value: "bar"},
						pci.SearchFlag(&policy.AllowScreenLock{}, pci.Tag),
						{Key: "ArcEnabled", Value: "" },
					},
				})
			}
			func test() {
				p := &policy.AllowDinosaurEasterEgg{}
				p2 := &policy.AllowScreenLock{}
				p3 := &policy.ArcEnabled{}
			}
			`,
	`package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)
func init() {
	testing.AddTest(&testing.Test{
		Params: []testing.Param{
			{
				Name: "autoclick",
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "autoclick",
						wantValue: true,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "autoclick",
						wantValue: false,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "autoclick",
						wantValue: false,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.AutoclickEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "caret_highlight",
				ExtraAttr: []string{"group:mainline"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "caretHighlight",
						wantValue: true,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "caretHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "caretHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.CaretHighlightEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
		},
	})
}
`,
	`package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)
func flags() []*testing.StringPair {
	var sp []*testing.StringPair
	sp = append(sp, &testing.StringPair{Key: &policy.AllowDinosaurEasterEgg{}, Value: "bar"})
	sp = append(sp, pci.SearchFlag(&policy.AllowScreenLock{}, pci.Tag))
	sp = append(sp, &testing.StringPair{Key: "ArcEnabled", Value: "bar"})
	return sp
}
func init() {
	testing.AddTest(&testing.Test{
		Func: test,
		SearchFlags: flags(),
	})
}
func test() {
	p := &policy.AllowDinosaurEasterEgg{}
	p2 := &policy.AllowScreenLock{}
	p3 := &policy.ArcEnabled{}
}
`,
	// Setting search flags from an imported function is not supported and the
	// check will skip the file.
	`package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
	"somepkg"
)
func init() {
	testing.AddTest(&testing.Test{
		Func: test,
		SearchFlags: somepkg.flags(),
	})
}
func test() {
	p := &policy.AllowDinosaurEasterEgg{}
	p2 := &policy.AllowScreenLock{}
	p}
`,
	// The check should skip a file that does not import the policy package.
	`package main
import (
	"chromiumos/tast/testing"
)
func init() {
	testing.AddTest(&testing.Test{
		Func: test,
	})
}
func test() {
}
`,
	// The check should skip a file that does not import the testing package.
	`package main
import (
	"chromiumos/tast/common/policy"
)
func somefunc() {
	p := &policy.AllowDinosaurEasterEgg{}
	p2 := &policy.AllowScreenLock{}
	p3 := &policy.ArcEnabled{}
}
`,
	// The check should skip a file that does not contain an init func.
	`package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)
func somefunc() {
	p := &policy.AllowDinosaurEasterEgg{}
	p2 := &policy.AllowScreenLock{}
	p3 := &policy.ArcEnabled{}
}
`,
	// The check should skip a file that does not declare a Tast test.
	`package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)
func init() {
}
func somefunc() {
	p := &policy.AllowDinosaurEasterEgg{}
	p2 := &policy.AllowScreenLock{}
	p3 := &policy.ArcEnabled{}
}
`,
}

func TestPCISearchFlag_ShouldReturnNil(t *testing.T) {
	for _, code := range testcode {
		f, fs := parse(code, "pcisearchflags_testfile.go")
		issues := SearchFlags(fs, f)
		verifyIssues(t, issues, nil)
	}
}

func TestPCISearchFlags_ShouldReturnOneIssue(t *testing.T) {
	const code = `package main
import (
	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)
func init() {
	testing.AddTest(&testing.Test{
		Func: test,
		SearchFlags: []*testing.StringPair{
			{Key: &policy.AllowDinosaurEasterEgg{}, Value: "bar"},
		},
	})
}
func test() {
	p := &policy.AllowScreenLock{}
}
`
	expects := []string{
		"pcisearchflags_testfile.go:15:8: Policy AllowScreenLock does not have a corresponding Search Flag.",
	}

	f, fs := parse(code, "pcisearchflags_testfile.go")
	issues := SearchFlags(fs, f)
	verifyIssues(t, issues, expects)
}
