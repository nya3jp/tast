// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"reflect"
	"strings"
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
			attrs: []string{"name:example.Pass", "bundle:cros", "dep:chrome"},
		},
		{
			attrs: []string{"disabled"},
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
			attrs: []string{"group:crosbolt", "disabled"},
		},
		{
			attrs: []string{"group:stress"},
		},
		{
			attrs: []string{"group:arc-data-collector"},
		},
		{
			attrs: []string{"group:arc-data-snapshot"},
		},
		{
			attrs: []string{"group:arc-functional"},
		},
		{
			attrs: []string{"group:appcompat"},
		},
		{
			attrs: []string{"group:appcompat", "appcompat_smoke"},
		},
		{
			attrs: []string{"group:graphics"},
		},
		{
			attrs: []string{"group:drivefs-cq"},
		},
		{
			attrs: []string{"group:enrollment"},
		},
		{
			attrs: []string{"group:dpanel-end2end"},
		},
		{
			attrs: []string{"group:input-tools"},
		},
		{
			attrs: []string{"group:input-tools-upstream"},
		},
		{
			attrs: []string{"group:flashrom"},
		},
		{
			attrs: []string{"group:hwsec_destructive_func"},
		},
		{
			attrs: []string{"group:hwsec_destructive_crosbolt", "hwsec_destructive_crosbolt_perbuild"},
		},
		{
			attrs: []string{"group:runtime_probe"},
		},
		{
			attrs: []string{"group:rapid-ime-decoder"},
		},
		{
			attrs: []string{"group:storage-qual"},
		},
		{
			attrs: []string{"group:wilco_bve"},
		},
		{
			attrs: []string{"group:wilco_bve_dock"},
		},

		// Invalid cases.
		{
			attrs: []string{""},
			error: `attribute "" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"informational"},
			error: `attribute "informational" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"informational", "disabled"},
			error: `attribute "informational" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"foo"},
			error: `attribute "foo" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"foo:bar"},
			error: `attribute "foo:bar" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:mainline", "foo"},
			error: `attribute "foo" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:foo"},
			error: `group "foo" is invalid; see chromiumos/tast/internal/testing/attr.go for the full list of valid groups`,
		},
		{
			attrs: []string{"group:crosbolt", "crosbolt_weekly", "informational"},
			error: `attribute "informational" is invalid in current groups; see chromiumos/tast/internal/testing/attr.go for the full list of valid attributes`,
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

func TestModifyAttrsForCompat(t *testing.T) {
	for _, tc := range []struct {
		orig []string
		want []string
	}{
		{nil, []string{"disabled"}},
		{[]string{"dep:chrome"}, []string{"dep:chrome", "disabled"}},
		{[]string{"group:mainline"}, []string{"group:mainline"}},
		{[]string{"group:mainline", "dep:chrome"}, []string{"group:mainline", "dep:chrome"}},
		{[]string{"group:foo"}, []string{"group:foo"}},
	} {
		attrs := append([]string(nil), tc.orig...)
		got := modifyAttrsForCompat(attrs)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("modifyAttrsForCompat(%q) = %q; want %q", tc.orig, got, tc.want)
		}
	}
}

func TestExtraAttributes(t *testing.T) {
	for _, g := range validGroups {
		prefix := g.Name + "_"
		for _, a := range g.Subattrs {
			// informational is an attribute that is allowed across multiple groups
			// to allow standardisation of test demotion operations by oncall.
			if a.Name == "informational" {
				continue
			}
			if !strings.HasPrefix(a.Name, prefix) {
				t.Errorf("Group %q has a subattribute %q but it should have the prefix %q", g.Name, a.Name, prefix)
			}
		}
	}
}
