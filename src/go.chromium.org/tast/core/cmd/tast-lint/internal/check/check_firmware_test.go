// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"testing"
)

const fwTestPath = "src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/do_stuff.go"
const fwInitTmpl = `package pkg

func init() {%v
}
`

func TestFirmwareParams(t *testing.T) {
	for _, tc := range []struct {
		snip    string
		wantMsg []string
	}{
		// Simple example of pd test.
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_pd"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr:         []string{"attr1"},
			ExtraSoftwareDeps: []string{"deps1", qualified.name},
			Val: firmware.PDTestParams{},
		}, {
			Name: "param2",
			Val: firmware.PDTestParams{},
		}},
	})`, nil},
		// Example of incorrect use of gate attrs
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec", "firmware_stressed"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr: []string{"firmware_meets_kpi"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"firmware_enabled"},
		}},
	})`, []string{fwTestPath + ":15:4: " + fmt.Sprintf(missingGateAttr, "firmware_enabled", "firmware_meets_kpi")}},
		// Example of gate attrs without firmware_ec
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_stressed"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr: []string{"firmware_meets_kpi"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
		}},
	})`, []string{
			fwTestPath + ":12:4: " + fmt.Sprintf(gateAttrWithoutRequiredAttr, "firmware_meets_kpi"),
			fwTestPath + ":15:4: " + fmt.Sprintf(gateAttrWithoutRequiredAttr, "firmware_enabled"),
			fwTestPath + ":15:4: " + fmt.Sprintf(gateAttrWithoutRequiredAttr, "firmware_meets_kpi"),
		}},
		// Example of correct use of gate attrs
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec", "firmware_stressed"},
		Params: []Param{{
			Name: "param1",
			ExtraAttr: []string{"firmware_meets_kpi"},
		}, {
			Name: "param2",
			ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
		}},
	})`, nil},
	} {
		code := fmt.Sprintf(fwInitTmpl, tc.snip)
		f, fs := parse(code, fwTestPath)
		issues := VerifyFirmwareAttrs(fs, f)
		verifyIssues(t, issues, tc.wantMsg)
	}
}
