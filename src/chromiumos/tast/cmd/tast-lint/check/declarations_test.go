// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

const declTestPath = "src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

func TestDeclarationsPass(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	verifyIssues(t, issues, nil)
}

func TestDeclarationsTopLevelAddTest(t *testing.T) {
	const code = `package pkg
func init() {
	for {
		testing.AddTest(&testing.Test{
			Func: DoStuff,
		})
	}
	testing.AddTest(ts)
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":4:3: " + notTopAddTestMsg,
		declTestPath + ":8:18: " + addTestArgLitMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsDesc(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		// Desc is missing
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     variableDesc,
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})
}
`

	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":3:18: " + noDescMsg,
		declTestPath + ":10:13: " + nonLiteralDescMsg,
		declTestPath + ":15:13: " + badDescMsg,
		declTestPath + ":20:13: " + badDescMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsContacts(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		// Contacts is missing
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{variableAddress},
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: variableContacts,
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":3:18: " + noContactMsg,
		declTestPath + ":11:22: " + nonLiteralContactsMsg,
		declTestPath + ":16:13: " + nonLiteralContactsMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsAttr(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"this", "is", "valid", "attr"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{variableAttr},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":13:13: " + nonLiteralAttrMsg,
		declTestPath + ":19:22: " + nonLiteralAttrMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsVars(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{"this", "is", "valid", "vars"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{variableVar},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":13:13: " + nonLiteralVarsMsg,
		declTestPath + ":19:22: " + nonLiteralVarsMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsSoftwareDeps(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{"this", "is", "valid", "dep"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{qualified.variable, is, "allowed"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{fun()},  // invocation is not allowed.
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":19:17: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":25:26: " + nonLiteralSoftwareDepsMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestDeclarationsParams(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr:         []string{"attr1"},
			ExtraSoftwareDeps: []string{"deps1", qualified.name},
		}, {
			Name: "param2",
		}},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   variableParams,
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   []Param{variableParamStruct},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
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
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f, false)
	expects := []string{
		declTestPath + ":19:13: " + nonLiteralParamsMsg,
		declTestPath + ":25:21: " + nonLiteralParamsMsg,
		declTestPath + ":32:10: " + nonLiteralParamNameMsg,
		declTestPath + ":34:23: " + nonLiteralAttrMsg,
		declTestPath + ":36:32: " + nonLiteralAttrMsg,
		declTestPath + ":38:23: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":40:32: " + nonLiteralSoftwareDepsMsg,
	}
	verifyIssues(t, issues, expects)
}

func TestAutoFixDeclarationDesc(t *testing.T) {
	files := make(map[string]string)
	files[declTestPath] = `package pkg

func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized and ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})
}
`
	expects := make(map[string]string)
	expects[declTestPath] = `package pkg

func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Not capitalized",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Not capitalized and ends with a period",
		Contacts: []string{"me@chromium.org"},
	})
}
`
	verifyAutoFix(t, Declarations, files, expects)
}
