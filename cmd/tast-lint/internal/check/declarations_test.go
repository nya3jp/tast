// Copyright 2019 The ChromiumOS Authors
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
		BugComponent: "b:1034625",
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
		BugComponent: "b:1034625",
	})`, declTestPath + ":4:18: " + noDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     variableDesc,
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
	})`, declTestPath + ":6:13: " + nonLiteralDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
	})`, declTestPath + ":6:13: " + badDescMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
	})`, declTestPath + ":6:13: " + badDescMsg}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, []string{tc.wantMsg})
	}
}

func TestDeclarationsBugComponent(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg []string
	}{{`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "b:123asdf",
	})`, []string{declTestPath + ":8:17: b:123asdf " + nonBugComponentMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "b:asdf123",
	})`, []string{declTestPath + ":8:17: b:asdf123 " + nonBugComponentMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "crbug:asdf<asdf",
	})`, []string{declTestPath + ":8:17: crbug:asdf<asdf " + nonBugComponentMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "TBA",
	})`, nil}, {`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "b:1034625",
	})`, nil}, {`
	testing.AddTest(&testing.Test{
		Func:	DoStuff,
		Desc:	"Litteral Desc",
		Contacts:  []string{"me@chromium.org"},
		BugComponent: "crbug:asdf>asdf",
	})`, nil}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, tc.wantMsg)
	}
}

func TestDeclarationsContacts(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg []string
	}{{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		BugComponent: "b:1034625",
		// Contacts is missing
	})`, []string{declTestPath + ":4:18: " + noContactMsg}}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{variableAddress},
		BugComponent: "b:1034625",
	})`, []string{declTestPath + ":7:22: " + nonLiteralContactsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: variableContacts,
		BugComponent: "b:1034625",
	})`, []string{declTestPath + ":7:13: " + nonLiteralContactsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{"mechromium.org"},
		BugComponent: "b:1034625",
	})`, []string{declTestPath + ":7:22: " + nonLiteralContactsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{"m@chromium.org@chromium.org"},
		BugComponent: "b:1034625",
	})`, []string{declTestPath + ":7:22: " + nonLiteralContactsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
	})`, nil}, {`
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{"me-me+sub@chromium.org"},
		BugComponent: "b:1034625",
	})`, nil}} {
		code := fmt.Sprintf(initTmpl, tc.snip)
		f, fs := parse(code, declTestPath)
		issues := TestDeclarations(fs, f, git.CommitFile{}, false)
		verifyIssues(t, issues, tc.wantMsg)
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
		BugComponent: "b:1034625",
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     foobar,  // non array literal.
		BugComponent: "b:1034625",
	})`, declTestPath + ":8:13: " + nonLiteralAttrMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{variableAttr},
		BugComponent: "b:1034625",
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
			BugComponent: "b:1034625",
			Vars:     []string{"this", "is", "valid", "vars"},
		})`}, {snip: `
		testing.AddTest(&testing.Test{
			Func:     DoStuff,
			Desc:     "This description is fine",
			Contacts: []string{"me@chromium.org"},
			BugComponent: "b:1034625",
			Vars:     append([]string{"this", "is", "valid", "vars", localConstant}, foo.BarList...),
		})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     append(foo.BarList, "this", "is", "valid", "vars", localConstant),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     foo.BarList,
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     append(foo.BarList, bar.Baz...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     []string{foo.BarConstant, localConstant},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     append(foo.BazList, localConstant),
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     append(foo.BarList, localList...),
	})`, declTestPath + ":9:13: " + nonLiteralVarsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     localList,
	})`, declTestPath + ":9:13: " + nonLiteralVarsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Vars:     append(localList, "foo", "bar"),
	})`, declTestPath + ":9:13: " + nonLiteralVarsMsg}} {
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
		BugComponent: "b:1034625",
		SoftwareDeps: []string{"this", "is", "valid", "dep"},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps: []string{qualified.variable, is, "allowed"},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     append([]string{"this", "is", "valid", localConstant}, foo.BarList...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     append(foo.BarList, "this", "is", "valid", localConstant),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     foo.BarList,
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     append(foo.BarList, bar.Baz...),
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     []string{foo.BarConstant, localConstant},
	})`}, {snip: `
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps:     append(foo.BazList, localConstant),
	})`}, {`
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps: foobar,  // non array literal.
	})`, declTestPath + ":10:17: " + nonLiteralSoftwareDepsMsg}, {`
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		SoftwareDeps: []string{fun()},  // invocation is not allowed.
	})`, declTestPath + ":10:17: " + nonLiteralSoftwareDepsMsg}} {
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
		BugComponent: "b:1034625",
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
		BugComponent: "b:1034625",
		Params:   variableParams,
	})`, []string{declTestPath + ":10:13: " + nonLiteralParamsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Params:   []Param{variableParamStruct},
	})`, []string{declTestPath + ":10:21: " + nonLiteralParamsMsg}}, {`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
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
		declTestPath + ":11:10: " + nonLiteralParamNameMsg,
		declTestPath + ":13:23: " + nonLiteralAttrMsg,
		declTestPath + ":15:32: " + nonLiteralAttrMsg,
		declTestPath + ":17:23: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":19:23: " + nonLiteralSoftwareDepsMsg,
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
