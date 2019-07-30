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
	}
	verifyIssues(t, issues, expects)
}
