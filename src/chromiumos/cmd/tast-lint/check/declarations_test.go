// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

const declTestPath = "/src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

func TestDeclarationsPass(t *testing.T) {
	const code = `package pkg
func init() {
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"group:mainline"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
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
	issues := Declarations(fs, f)
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
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     variableDesc,
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"group:mainline"},
	})
}
`

	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":3:18: " + noDescMsg,
		declTestPath + ":11:13: " + nonLiteralDescMsg,
		declTestPath + ":17:13: " + badDescMsg,
		declTestPath + ":23:13: " + badDescMsg,
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
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: []string{variableAddress},
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "This description is fine",
		Contacts: variableContacts,
		Attr:     []string{"group:mainline"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":3:18: " + noContactMsg,
		declTestPath + ":12:22: " + nonLiteralContactsMsg,
		declTestPath + ":18:13: " + nonLiteralContactsMsg,
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
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     nil,
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		// Attr is missing.
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":13:13: " + nonLiteralAttrMsg,
		declTestPath + ":19:22: " + nonLiteralAttrMsg,
		declTestPath + ":25:13: " + emptyAttrMsg,
		declTestPath + ":31:13: " + emptyAttrMsg,
		declTestPath + ":33:18: " + noAttrMsg,
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
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     foobar,  // non array literal.
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{variableVar},
		Attr:     []string{"group:mainline"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":14:13: " + nonLiteralVarsMsg,
		declTestPath + ":21:22: " + nonLiteralVarsMsg,
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
		Attr:         []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{qualified.variable, is, "allowed"},
		Attr:         []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: foobar,  // non array literal.
		Attr:         []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuff,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{fun()},  // invocation is not allowed.
		Attr:         []string{"group:mainline"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":21:17: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":28:26: " + nonLiteralSoftwareDepsMsg,
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
		Attr: []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   variableParams,
		Attr:     []string{"group:mainline"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   []Param{variableParamStruct},
		Attr:     []string{"group:mainline"},
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
		Attr: []string{"group:mainline"},
	})
}
`
	f, fs := parse(code, declTestPath)
	issues := Declarations(fs, f)
	expects := []string{
		declTestPath + ":20:13: " + nonLiteralParamsMsg,
		declTestPath + ":27:21: " + nonLiteralParamsMsg,
		declTestPath + ":35:10: " + nonLiteralParamNameMsg,
		declTestPath + ":37:23: " + nonLiteralAttrMsg,
		declTestPath + ":39:32: " + nonLiteralAttrMsg,
		declTestPath + ":41:23: " + nonLiteralSoftwareDepsMsg,
		declTestPath + ":43:32: " + nonLiteralSoftwareDepsMsg,
	}
	verifyIssues(t, issues, expects)
}
