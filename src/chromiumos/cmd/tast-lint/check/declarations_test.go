// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
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
			Func: DoStuffA,
		})
	}
	testing.AddTest(ts)

	// Desc verification.
	testing.AddTest(&testing.Test{
		Func:     DoStuffB,
		// Desc is missing
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffC,
		Desc:     variableDesc,
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffD,
		Desc:     "not capitalized",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffE,
		Desc:     "Ends with a period.",
		Contacts: []string{"me@chromium.org"},
	})

	// Contacts verification.
	testing.AddTest(&testing.Test{
		Func:     DoStuffF,
		Desc:     "This description is fine",
		// Contacts is missing
	})
	testing.AddTest(&testing.Test{
		Func: DoStuffG,
		Desc: "This description is fine",
		Contacts: []string{variableAddress},
	})
	testing.AddTest(&testing.Test{
		Func: DoStuffH,
		Desc: "This description is fine",
		Contacts: variableContacts,
	})

	// Attr verification.
	testing.AddTest(&testing.Test{
		Func:     DoStuffI,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{"this", "is", "valid", "attr"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffJ,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffK,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Attr:     []string{variableAttr},
	})

	// Vars verification.
	testing.AddTest(&testing.Test{
		Func:     DoStuffL,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{"this", "is", "valid", "vars"},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffM,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffN,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Vars:     []string{variableVar},
	})

	// SoftwareDeps verification.
	testing.AddTest(&testing.Test{
		Func:         DoStuffO,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{"this", "is", "valid", "dep"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuffP,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{qualified.variable, is, "allowed"},
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuffQ,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: foobar,  // non array literal.
	})
	testing.AddTest(&testing.Test{
		Func:         DoStuffR,
		Desc:         "This description is fine",
		Contacts:     []string{"me@chromium.org"},
		SoftwareDeps: []string{fun()},  // invocation is not allowed.
	})

	// Params verification.
	testing.AddTest(&testing.Test{
		Func:     DoStuffS,
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
		Func:     DoStuffT,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   variableParams,
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffU,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		Params:   []Param{variableParamStruct},
	})
	testing.AddTest(&testing.Test{
		Func:     DoStuffV,
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
	// Pass case now fails because repeated reference
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
	testing.AddTest(&testing.Test{
		Func:     doStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
	})
}

func DoStuff(ctx context.Context, s *testing.State) {}
func DoStuffA(ctx context.Context, s *testing.State) {}
func DoStuffB(ctx context.Context, s *testing.State) {}
func DoStuffC(ctx context.Context, s *testing.State) {}
func DoStuffD(ctx context.Context, s *testing.State) {}
func DoStuffE(ctx context.Context, s *testing.State) {}
func DoStuffF(ctx context.Context, s *testing.State) {}
func DoStuffG(ctx context.Context, s *testing.State) {}
func DoStuffH(ctx context.Context, s *testing.State) {}
func DoStuffI(ctx context.Context, s *testing.State) {}
func DoStuffJ(ctx context.Context, s *testing.State) {}
func DoStuffK(ctx context.Context, s *testing.State) {}
func DoStuffL(ctx context.Context, s *testing.State) {}
func DoStuffM(ctx context.Context, s *testing.State) {}
func DoStuffN(ctx context.Context, s *testing.State) {}
func DoStuffO(ctx context.Context, s *testing.State) {}
func DoStuffP(ctx context.Context, s *testing.State) {}
func DoStuffQ(ctx context.Context, s *testing.State) {}
func DoStuffR(ctx context.Context, s *testing.State) {}
func DoStuffS(ctx context.Context, s *testing.State) {}
func DoStuffT(ctx context.Context, s *testing.State) {}
func DoStuffU(ctx context.Context, s *testing.State) {}
func DoStuffV(ctx context.Context, s *testing.State) {}
func doStuff(ctx context.Context, s *testing.State) {}
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
		path + ":176:13: " + repeatedTestFuncMsg,
		path + ":181:13: " + notFoundFuncMsg,
		path + ":188:1: " + fmt.Sprintf(unusedFuncMsg, "DoStuffA"),
	}
	verifyIssues(t, issues, expects)
}
