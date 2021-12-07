// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package externalservers_test

import (
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/internal/minidriver/externalservers"
)

// TestParseServerVarValues makes sure the parsing of the values of
// server related run time variables returns expected values.
// and return a role to server host map.
// Example input: ":addr1:22,cd1:addr2:2222"
// Example output: { "": "addr1:22", "cd1": "addr2:2222" }
func TestParseServerVarValues(t *gotesting.T) {
	type testData struct {
		input  string
		wanted map[string]string
	}
	tests := []testData{
		{
			input:  "",
			wanted: map[string]string{},
		},
		{
			input:  ":addr_prim:22,cd1:addr1:2221,cd2:addr2:2222",
			wanted: map[string]string{"": "addr_prim:22", "cd1": "addr1:2221", "cd2": "addr2:2222"},
		},
		{
			input:  ":addr_prim:9999:ssh:22,cd1:127.0.0.1:2221:ssh:22",
			wanted: map[string]string{"": "addr_prim:9999:ssh:22", "cd1": "127.0.0.1:2221:ssh:22"},
		},
		{
			input:  ":addr_prim,cd1:addr1",
			wanted: map[string]string{"": "addr_prim", "cd1": "addr1"},
		},
	}
	for _, test := range tests {
		got, err := externalservers.ParseServerVarValues(test.input)
		if err != nil {
			t.Errorf("Error in parsing %q: %v", test.input, err)
			continue
		}
		if diff := cmp.Diff(got, test.wanted, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("failed get expected host info (-got +want):\n%s", diff)
		}
	}
}

// TestParseServerVarValuesBad makes sure ParseServerVarValues handle
// illegal values.
func TestParseServerVarValuesBad(t *gotesting.T) {
	tests := []string{
		":addr_pri:22,,cd2:addr2:2222",
		"no_role1,no_role2",
		",",
	}
	for _, test := range tests {
		if _, err := externalservers.ParseServerVarValues(test); err == nil {
			t.Errorf("Did not get error in parsing %q", test)
			continue
		}
	}
}
