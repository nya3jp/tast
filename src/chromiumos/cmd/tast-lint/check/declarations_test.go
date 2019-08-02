// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestDeclaration(t *testing.T) {
	const code = `package pkg

import (
	"context"

	"chromiumos/tast/testing"
)

func init() {
	// We wouldn't normally permit multiple AddTest calls like this, but including
	// them here makes this unit test shorter.

	// Simple pass case.
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})

	// AddTest invocation tests.
	for {
		testing.AddTest(&testing.Test{
			Func: DoStuff,
		})
	}
	testing.AddTest(ts)

	// Desc verification.
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

	// Contacts verification.
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

	// Attr verification.
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

	// Vars verification.
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

	// SoftwareDeps verification.
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

	// Params verification.
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

func DoStuff(ctx context.Context, s *testing.State) {}
`

	const path = "/src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

	f, fs := parse(code, path)

	issues := Declarations(fs, f)
	expects := []string{
		path + ":22:3: " + notTopAddTestMsg,
		path + ":26:18: " + addTestArgLitMsg,

		path + ":29:18: " + noDescMsg,
		path + ":36:13: " + nonLiteralDescMsg,
		path + ":41:13: " + badDescMsg,
		path + ":46:13: " + badDescMsg,

		path + ":51:18: " + noContactMsg,
		path + ":59:22: " + nonLiteralContactsMsg,
		path + ":64:13: " + nonLiteralContactsMsg,

		path + ":78:13: " + nonLiteralAttrMsg,
		path + ":84:22: " + nonLiteralAttrMsg,

		path + ":98:13: " + nonLiteralVarsMsg,
		path + ":104:22: " + nonLiteralVarsMsg,

		path + ":124:17: " + nonLiteralSoftwareDepsMsg,
		path + ":130:26: " + nonLiteralSoftwareDepsMsg,

		path + ":150:13: " + nonLiteralParamsMsg,
		path + ":156:21: " + nonLiteralParamsMsg,
		path + ":163:10: " + nonLiteralParamNameMsg,
		path + ":165:23: " + nonLiteralAttrMsg,
		path + ":167:32: " + nonLiteralAttrMsg,
		path + ":169:23: " + nonLiteralSoftwareDepsMsg,
		path + ":171:32: " + nonLiteralSoftwareDepsMsg,
	}
	verifyIssues(t, issues, expects)
}
