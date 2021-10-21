// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"

	"go.chromium.org/tast/cmd/tast-lint/internal/git"
)

const declTestPath = "src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

func TestDeclarationsPass(t *testing.T) {
	const code = `package pkg
func init() {
	// Comments are allowed.
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
}

var x SomeComplexStruct

// init without AddTest should be allowed.
func init() {
	x = f()
}
`
	f, fs := parse(code, declTestPath)
	issues := TestDeclarations(fs, f, git.CommitFile{}, false)
	verifyIssues(t, issues, nil)
}

const initTmpl = `package pkg

func init() {%v
}
`

func TestDeclarationsOnlyTopLevelAddTest(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{`
	for {
		testing.AddTest(&testing.Test{
			Func:     DoStuff,
			Desc:     "This description is fine",
			Contacts: []string{"me@chromium.org"},
		})
	}`, declTestPath + ":5:3: " + notOnlyTopAddTestMsg}, {`
	_ = 1
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
	for {}`, declTestPath + ":5:2: " + notOnlyTopAddTestMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})`, declTestPath + ":4:2: " + notOnlyTopAddTestMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, []string{tc.wantMsg})
	}
}

func TestDeclarationsDesc(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		// Desc is missing
		Contacts: []string{"me@chromium.org"},
	})`, declTestPath + ":4:18: " + noDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     variableDesc,
		Contacts: []string{"me@chromium.org"},
	})`, declTestPath + ":6:13: " + nonLiteralDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
	})`, declTestPath + ":6:13: " + badDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})`, declTestPath + ":6:13: " + badDescMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, []string{tc.wantMsg})
	}
}

func TestDeclarationsContacts(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		// Contacts is missing
	})`, declTestPath + ":4:18: " + noContactMsg}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{variableAddress},
	})`, declTestPath + ":7:22: " + nonLiteralContactsMsg}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: variableContacts,
	})`, declTestPath + ":7:13: " + nonLiteralContactsMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, []string{tc.wantMsg})
	}
}

func TestDeclarationsAttr(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"this", "is", "valid", "attr"},
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     foobar,  // non array literal.
	})`, declTestPath + ":8:13: " + nonLiteralAttrMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{variableAttr},
	})`, declTestPath + ":8:22: " + nonLiteralAttrMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		var expects []string
		if tc.wantMsg != "" {
			expects = append(expects, tc.wantMsg)
		}
		verifyIssues(t, issues, expects)
	}
}

func TestDeclarationsVars(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{snip: `
		testing.AddTest(&testing.Test{
			Func:     DoStuff,
			Desc:     "This description is fine",
			Contacts: []string{"me@chromium.org"},
			Vars:     []string{"this", "is", "valid", "vars"},
		})`}, {snip: `
		testing.AddTest(&testing.Test{
			Func:     DoStuff,
			Desc:     "This description is fine",
			Contacts: []string{"me@chromium.org"},
			Vars:     append([]string{"this", "is", "valid", "vars", localConstant}, foo.BarList...),
		})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     append(foo.BarList, "this", "is", "valid", "vars", localConstant),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     foo.BarList,
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     append(foo.BarList, bar.Baz...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{foo.BarConstant, localConstant},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     append(foo.BazList, localConstant),
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     append(foo.BarList, localList...),
	})`, declTestPath + ":8:13: " + nonLiteralVarsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     localList,
	})`, declTestPath + ":8:13: " + nonLiteralVarsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     append(localList, "foo", "bar"),
	})`, declTestPath + ":8:13: " + nonLiteralVarsMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		var expects []string
		if tc.wantMsg != "" {
			expects = append(expects, tc.wantMsg)
		}
		verifyIssues(t, issues, expects)
	}
}

func TestDeclarationsSoftwareDeps(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg string
	}{{snip: `
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{"this", "is", "valid", "dep"},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{qualified.variable, is, "allowed"},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     append([]string{"this", "is", "valid", localConstant}, foo.BarList...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     append(foo.BarList, "this", "is", "valid", localConstant),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     foo.BarList,
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     append(foo.BarList, bar.Baz...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     []string{foo.BarConstant, localConstant},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		SoftwareDeps:     append(foo.BazList, localConstant),
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: foobar,  // non array literal.
	})`, declTestPath + ":9:17: " + nonLiteralSoftwareDepsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{fun()},  // invocation is not allowed.
	})`, declTestPath + ":9:17: " + nonLiteralSoftwareDepsMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		var expects []string
		if tc.wantMsg != "" {
			expects = append(expects, tc.wantMsg)
		}
		verifyIssues(t, issues, expects)
	}
}

func TestDeclarationsParams(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg []string
	}{{snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr:         []string{"attr1"},
			ExtraSoftwareDeps: []string{"deps1", qualified.name},
		}, {
			Name: "param2",
		}},
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   variableParams,
	})`, []string{declTestPath + ":9:13: " + nonLiteralParamsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   []Param{variableParamStruct},
	})`, []string{declTestPath + ":9:21: " + nonLiteralParamsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params: []Param{{
			Name: variableParamName,
		}, {
			ExtraAttr:         variableAttrs,
		}, {
			ExtraAttr:         []string{variableAttr},
		}, {
			ExtraSoftwareDeps: variableSoftwareDeps,
		}, {
			ExtraSoftwareDeps: []string{fun()},
		}},
	})`, []string{
		declTestPath + ":10:10: " + nonLiteralParamNameMsg,
		declTestPath + ":12:23: " + nonLiteralAttrMsg,
		declTestPath + ":14:32: " + nonLiteralAttrMsg,
		declTestPath + ":16:23: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":18:23: " + nonLiteralSoftwareDepsMsg,
	}}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, tc.wantMsg)
	}
}

func TestAutoFixDeclarationDesc(t *testing.T) {
	for _, tc := range []struct {
		cur, want string
	}{{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
	})`, `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Not capitalized",
		Contacts: []string{"me@chromium.org"},
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})`, `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period",
		Contacts: []string{"me@chromium.org"},
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized and ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})`, `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Not capitalized and ends with a period",
		Contacts: []string{"me@chromium.org"},
	})`}} {
		verifyAutoFix(t, func(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
			return TestDeclarations(fs, f, git.CommitFile{}, fix)
		},
			map[string]string{declTestPath: fmt.Sprintf(initTmpl, tc.cur)},
			map[string]string{declTestPath: fmt.Sprintf(initTmpl, tc.want)})
	}
}

func TestFixtureDeclarationsPass(t *testing.T) {
	const code = `package pkg

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "fixtA",
		Desc:            "Valid description",
		Contacts:        []string{"me@chromium.org"},
		Impl:            &fakeFixture{param: "A"},
		SetUpTimeout:    fixtureTimeout,
		TearDownTimeout: fixtureTimeout,
		Vars:            []string{"foo", bar},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "anotherFixt",
		Desc:     "Valid description",
		Contacts: []string{"me@chromium.org"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := FixtureDeclarations(fs, f, false)
	verifyIssues(t, issues, nil)
}

func TestFixtureDeclarationsFailure(t *testing.T) {
	const code = `package pkg

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "fixtA",
		Impl: &fakeFixture{param: "A"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "fixtB",
		Contacts: contacts,
		Desc:     desc,
		Impl:     &fakeFixture{param: "B"},
		Vars:     vars,
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := FixtureDeclarations(fs, f, false)
	verifyIssues(t, issues, []string{
		declTestPath + ":4:21: " + noDescMsg,
		declTestPath + ":4:21: " + noContactMsg,
		declTestPath + ":10:13: " + nonLiteralContactsMsg,
		declTestPath + ":11:13: " + nonLiteralDescMsg,
		declTestPath + ":13:13: " + nonLiteralVarsMsg,
	})
}
