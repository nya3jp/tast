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
		Func: DoStuff,
		Desc: "This description is fine",
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "not capitalized",
	})
	testing.AddTest(&testing.Test{
		Func: DoStuff,
		Desc: "Ends with a period.",
	})
}

func DoStuff(ctx context.Context, s *testing.State) {}
`

	const path = "/src/chromiumos/tast/local/bundles/cros/example/do_stuff.go"

	f, fs := parse(code, path)

	issues := Declarations(fs, f)
	expects := []string{
		path + ":18:9: " + badDescMsg,
		path + ":22:9: " + badDescMsg,
	}
	verifyIssues(t, issues, expects)
}
