// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"
)

func TestCheckKnownAttrs(t *testing.T) {
	for _, tc := range []struct {
		attrs []string
		error string
	}{
		// Valid cases.
		{
			attrs: nil,
		},
		{
			attrs: []string{"informational"},
		},
		{
			attrs: []string{"disabled"},
		},
		{
			attrs: []string{"informational", "disabled"},
		},
		{
			attrs: []string{"group:mainline"},
		},
		{
			attrs: []string{"group:mainline", "informational"},
		},
		{
			attrs: []string{"group:mainline", "disabled"},
		},
		{
			attrs: []string{"group:mainline", "informational", "disabled"},
		},
		{
			attrs: []string{"group:crosbolt"},
		},
		{
			attrs: []string{"group:crosbolt", "crosbolt_weekly"},
		},
		{
			attrs: []string{"group:stress"},
		},
		{
			attrs: []string{"group:appcompat"},
		},
		{
			attrs: []string{"group:enrollment"},
		},
		{
			attrs: []string{"group:flashrom"},
		},
		{
			attrs: []string{"group:runtime_probe"},
		},

		// Invalid cases.
		{
			attrs: []string{""},
			error: `attribute "" is invalid in current groups; see chromiumos/tast/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"foo"},
			error: `attribute "foo" is invalid in current groups; see chromiumos/tast/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:mainline", "foo"},
			error: `attribute "foo" is invalid in current groups; see chromiumos/tast/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:foo"},
			error: `group "foo" is invalid; see chromiumos/tast/testing/attr.go for the full list of valid groups`,
		},
		{
			attrs: []string{"group:crosbolt", "crosbolt_weekly", "informational"},
			error: `attribute "informational" is invalid in current groups; see chromiumos/tast/testing/attr.go for the full list of valid attributes`,
		},
	} {
		err := checkKnownAttrs(tc.attrs)
		if tc.error == "" {
			if err != nil {
				t.Errorf("checkKnownAttrs(%+v) unexpectedly failed: %v", tc.attrs, err)
			}
		} else {
			if err == nil {
				t.Errorf("checkKnownAttrs(%+v) unexpectedly succeeded; want %q", tc.attrs, tc.error)
			} else if err.Error() != tc.error {
				t.Errorf("checkKnownAttrs(%+v) returned %q; want %q", tc.attrs, err.Error(), tc.error)
			}
		}
	}
}
