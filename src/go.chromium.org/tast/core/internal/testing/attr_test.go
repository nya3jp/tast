// Copyright 2019 The ChromiumOS Authors
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
			attrs: []string{"group:appcompat", "appcompat_default"},
		},
		{
			attrs: []string{"group:appcompat", "appcompat_smoke"},
		},
		{
			attrs: []string{"group:appcompat", "appcompat_top_apps"},
		},
		{
			attrs: []string{"group:arcappgameperf"},
		},
		{
			attrs: []string{"group:arcappmediaperf"},
		},
		{
			attrs: []string{"group:arc", "arc_playstore"},
		},
		{
			attrs: []string{"group:arc", "arc_core"},
		},
		{
			attrs: []string{"group:arc", "arc_chromeos_vm"},
		},
		{
			attrs: []string{"group:camera", "camera_cca"},
		},
		{
			attrs: []string{"group:camera", "camera_service"},
		},
		{
			attrs: []string{"group:camera", "camera_hal"},
		},
		{
			attrs: []string{"group:camera", "camera_kernel"},
		},
		{
			attrs: []string{"group:camera", "camera_functional"},
		},
		{
			attrs: []string{"group:camera", "camera_pnp"},
		},
		{
			attrs: []string{"group:camera", "camera_config"},
		},
		{
			attrs: []string{"group:cq-minimal"},
		},
		{
			attrs: []string{"group:crostini_slow"},
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
			attrs: []string{"group:data-leak-prevention-dmserver-enrollment-daily"},
		},
		{
			attrs: []string{"group:dmserver-enrollment-daily"},
		},
		{
			attrs: []string{"group:powerwash-daily"},
		},
		{
			attrs: []string{"group:meet-powerwash-daily"},
		},
		{
			attrs: []string{"group:dmserver-enrollment-live"},
		},
		{
			attrs: []string{"group:dmserver-zteenrollment-daily"},
		},
		{
			attrs: []string{"group:dpanel-end2end"},
		},
		{
			attrs: []string{"group:enterprise-reporting"},
		},
		{
			attrs: []string{"group:enterprise-reporting-daily"},
		},
		{
			attrs: []string{"group:external-dependency"},
		},
		{
			attrs: []string{"group:inputs_appcompat_arc_perbuild"},
		},
		{
			attrs: []string{"group:inputs_orca_daily"},
		},
		{
			attrs: []string{"group:inputs_appcompat_citrix_perbuild"},
		},
		{
			attrs: []string{"group:inputs_appcompat_gworkspace_perbuild"},
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
			attrs: []string{"group:language_packs_hw_recognition_dlc_download_daily"},
		},
		{
			attrs: []string{"group:launcher_search_quality_daily"},
		},
		{
			attrs: []string{"group:launcher_search_quality_per_build"},
		},
		{
			attrs: []string{"group:launcher_image_search_perbuild"},
		},
		{
			attrs: []string{"group:rapid-ime-decoder"},
		},
		{
			attrs: []string{"group:racc", "racc_general"},
		},
		{
			attrs: []string{"group:racc", "racc_config_installed"},
		},
		{
			attrs: []string{"group:racc", "racc_encrypted_config_installed"},
		},
		{
			attrs: []string{"group:storage-qual"},
		},
		{
			attrs: []string{"group:telemetry_extension_hw"},
		},
		{
			attrs: []string{"group:wilco_bve_dock"},
		},
		{
			attrs: []string{"group:criticalstaging"},
		},
		{
			attrs: []string{"group:cuj"},
		},
		{
			attrs: []string{"group:cuj", "cuj_experimental"},
		},
		{
			attrs: []string{"group:cuj", "cuj_weekly"},
		},
		{
			attrs: []string{"group:cuj", "cuj_loginperf"},
		},
		{
			attrs: []string{"group:healthd", "healthd_perbuild"},
		},
		{
			attrs: []string{"group:healthd", "healthd_weekly"},
		},
		{
			attrs: []string{"group:heartd", "heartd_perbuild"},
		},
		{
			attrs: []string{"group:privacyhub-golden"},
		},
		{
			attrs: []string{"group:ddd_test_group"},
		},
		{
			attrs: []string{"group:floatingworkspace"},
		},
		{
			attrs: []string{"group:typec"},
		},
		{
			attrs: []string{"group:meet"},
		},
		{
			attrs: []string{"group:meet", "informational"},
		},
		{
			attrs: []string{"group:crospts"},
		},
		{
			attrs: []string{"group:crospts", "crospts_x86"},
		},
		{
			attrs: []string{"group:crospts", "crospts_arm64"},
		},
		{
			attrs: []string{"group:sw_gates_virt", "sw_gates_virt_enabled"},
		},
		{
			attrs: []string{"group:sw_gates_virt", "sw_gates_virt_kpi"},
		},
		{
			attrs: []string{"group:sw_gates_virt", "sw_gates_virt_stress"},
		},
		{
			attrs: []string{"group:video_conference_face_framing_per_build"},
		},
		{
			attrs: []string{"group:fwupd"},
		},
		{
			attrs: []string{"group:demo-mode"},
		},

		// Invalid cases.
		{
			attrs: []string{""},
			error: `attribute "" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"informational"},
			error: `attribute "informational" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"informational", "disabled"},
			error: `attribute "informational" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"foo"},
			error: `attribute "foo" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"foo:bar"},
			error: `attribute "foo:bar" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:mainline", "foo"},
			error: `attribute "foo" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
		},
		{
			attrs: []string{"group:foo"},
			error: `group "foo" is invalid; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid groups`,
		},
		{
			attrs: []string{"group:crosbolt", "crosbolt_weekly", "informational"},
			error: `attribute "informational" is invalid in current groups; see go.chromium.org/tast/core/internal/testing/attr.go for the full list of valid attributes`,
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
