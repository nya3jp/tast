// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/token"
	"testing"

	"golang.org/x/exp/slices"
)

func TestAttrCheckerPass(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{"group:mainline", "informational"},
	})
}
`
	const path = "/src/chromiumos/tast/local/bundles/cros/example/keyboard.go"
	f, fs := parse(code, path)
	issues := checkAttr(fs, f, func(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
		return nil
	})
	verifyIssues(t, issues, nil)
}

func TestAttrCheckerFail(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{"group:mainline", "informational"},
	})
}
`
	const path = "/src/chromiumos/tast/local/bundles/cros/example/keyboard.go"
	f, fs := parse(code, path)
	issues := checkAttr(fs, f,
		func(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
			return []*Issue{
				{
					Pos: attrPos,
					Msg: "First issue.",
				},
				{
					Pos: attrPos,
					Msg: "Second issue.",
				},
			}
		},
	)
	expects := []string{
		path + ":4:3: First issue.",
		path + ":4:3: Second issue.",
	}
	verifyIssues(t, issues, expects)
}

func TestAttrCheckerExtraAttr(t *testing.T) {
	const code = `package main
func init() {
	testing.AddTest(&testing.Test{
		Params: []testing.Param{{
			Name: "param1",
			ExtraAttr: []string{"pass"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"fail"},
		}},
	})
}
`
	const path = "/src/chromiumos/tast/local/extra_attr.go"
	f, fs := parse(code, path)
	issues := checkAttr(fs, f,
		func(attrs []string, attrPos token.Position, requirements []string, requirementPos token.Position) []*Issue {
			if slices.Contains(attrs, "pass") {
				return nil
			}
			return []*Issue{{
				Pos: attrPos,
				Msg: "Failed.",
			}}
		},
	)
	expects := []string{
		path + ":9:4: Failed.",
	}
	verifyIssues(t, issues, expects)
}
