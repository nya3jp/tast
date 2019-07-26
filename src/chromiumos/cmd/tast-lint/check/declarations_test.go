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
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
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
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		// Contacts is missing
	})

	for {
		testing.AddTest(&testing.Test{
			Func: DoStuff,
		})
	}
	testing.AddTest(ts)
}

func DoStuff(ctx context.Context, s *testing.State) {}
`

	const path = "/src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

	f, fs := parse(code, path)

	issues := Declarations(fs, f)
	expects := []string{
		path + ":19:13: " + badDescMsg,
		path + ":24:13: " + badDescMsg,
		path + ":27:18: " + noContactMsg,
		path + ":34:3: " + notTopAddTestMsg,
		path + ":38:18: " + addTestArgLitMsg,
	}
	verifyIssues(t, issues, expects)
}
