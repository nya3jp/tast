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
	for index, tc := range []struct {
		snip    string
		wantMsg []string
	}{
		// [0] Simple example of pd test.
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_pd", "firmware_ec_ro", "firmware_ec_rw"},
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
		// [1] Example of incorrect use of gate attrs
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
		// [2] Example of gate attrs without firmware_ec
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
			fwTestPath + ":12:4: " + fmt.Sprintf(secondaryAttrWithoutRequiredAttr, "firmware_meets_kpi"),
			fwTestPath + ":15:4: " + fmt.Sprintf(secondaryAttrWithoutRequiredAttr, "firmware_enabled"),
			fwTestPath + ":15:4: " + fmt.Sprintf(secondaryAttrWithoutRequiredAttr, "firmware_meets_kpi"),
		}},
		// [3] Example of correct use of gate attrs
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
		// [4] firmware_bios_ro without firmware_bios
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_bios_ro"},
	})`, []string{
			fwTestPath + ":9:3: " + secondaryAttrWithoutBios,
		}},
		// [5] firmware_bios_rw without firmware_bios
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_bios_rw"},
	})`, []string{
			fwTestPath + ":9:3: " + biosRWWithoutRO,
			fwTestPath + ":9:3: " + secondaryAttrWithoutBios,
		}},
		// [6] firmware_bios_pdc without firmware_pd
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_bios_pdc"},
	})`, []string{
			fwTestPath + ":9:3: " + pdcWithoutPd,
		}},
		// [7] firmware_ec_ro without firmware_ec
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec_ro"},
	})`, []string{
			fwTestPath + ":9:3: " + secondaryAttrWithoutECPD,
		}},
		// [8] firmware_ec_rw without firmware_ec
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec_rw"},
	})`, []string{
			fwTestPath + ":9:3: " + ecRWWithoutRO,
			fwTestPath + ":9:3: " + secondaryAttrWithoutECPD,
		}},
		// [9] firmware_ec_rw with firmware_pd
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec_rw", "firmware_pd"},
	})`, []string{
			fwTestPath + ":9:3: " + ecRWWithoutRO,
			fwTestPath + ":9:3: " + pdWithoutSecondaryAttr,
		}},
		// [10] firmware_ec_ro with firmware_pd
		{`
	testing.AddTest(&testing.Test{
		Func:     DoStuff,
		Desc:     "This description is fine",
		Contacts: []string{"me@chromium.org"},
		BugComponent: "b:1034625",
		Attr: []string{"group:firmware", "firmware_ec_ro", "firmware_pd"},
	})`, nil},
		// [11] pd test without ro,rw attrs.
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
	})`, []string{
			fwTestPath + ":12:4: " + pdWithoutSecondaryAttr,
			fwTestPath + ":15:6: " + pdWithoutSecondaryAttr,
		}},
	} {
		code := fmt.Sprintf(fwInitTmpl, tc.snip)
		f, fs := parse(code, fwTestPath)
		issues := VerifyFirmwareAttrs(fs, f)
		verifyIssuesWithIndex(t, index, issues, tc.wantMsg)
	}
}
