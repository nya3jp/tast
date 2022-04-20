// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"strings"
)

// group defines a group of tests having the same purpose. For example, the mainline
// group contains functional tests to be used for build verification.
//
// A group definition should be agnostic to how it is scheduled in infrastructure.
// If needed, extra attributes can be defined to give hints for scheduling.
type group struct {
	// Name is the name of the group. A test can declare to belong to a group by
	// adding an attribute "group:<name>".
	Name string

	// Contacts is a list of email addresses of persons and groups responsible for
	// maintaining the test group.
	Contacts []string

	// Desc is a description of the group.
	Desc string

	// Subattrs defines extra attributes that can be used to annotate the tests
	// in the group.
	Subattrs []*attr
}

// attr defines an extra attribute to annotate tests.
//
// Attributes can give hint for scheduling.
type attr struct {
	// Name is the name of the attribute.
	Name string

	// Desc is a description of the attribute.
	Desc string
}

// validGroups is the list of all valid groups.
var validGroups = []*group{
	{
		Name:     "mainline",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `The default group of functional tests.

Mainline tests are run for build verification. Among others, pre-submit and
post-submit testing in Chrome OS CI and Chromium CI are important places where
mainlines tests are run.
`,
		Subattrs: []*attr{
			{
				Name: "informational",
				Desc: `Indicates that failures can be ignored.

Mainline tests lacking this attribute are called critical tests. Failures in
critical tests justify rejecting or reverting the responsible change, while
failures in informational tests do not.

All mainline tests should be initially marked informational, and should be
promoted to critical tests after stabilization.
`,
			},
		},
	},
	{
		Name:     "crosbolt",
		Contacts: []string{"crosbolt-eng@google.com"},
		Desc: `The group of performance tests to be run regularly by the crosbolt team.

Tests in this group are not used for build verification.
`,
		Subattrs: []*attr{
			{
				Name: "crosbolt_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
			{
				Name: "crosbolt_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "crosbolt_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
			{
				Name: "crosbolt_memory_nightly",
				Desc: `Indicates that this test is a memory test and should run nightly.`,
			},
		},
	},
	{
		Name:     "graphics",
		Contacts: []string{"chromeos-gfx@google.com", "chromeos-gfx-video@google.com"},
		Desc: `The group of graphics tests to be run regularly by the graphics team.

Tests in this group are not used for build verification.
`,
		Subattrs: []*attr{
			{
				Name: "graphics_trace",
				Desc: `Indicate this test is replaying a trace to reproduce graphics command.`,
			},
			{
				Name: "graphics_video",
				Desc: `Indicate this test is focus on video encode/decode.`,
			},
			{
				Name: "graphics_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
			{
				Name: "graphics_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "graphics_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
			{
				Name: "graphics_av_analysis",
				Desc: `Indicates that this test should run on audio/video analysis pool.`,
			},
			{
				Name: "graphics_drm",
				Desc: `Indicates that this test is part of DRM testing.`,
			},
			{
				Name: "graphics_igt",
				Desc: `Indicates that this test is running igt-gpu-tools.`,
			},
			{
				Name: "graphics_opencl",
				Desc: `Indicates that this test is part of OpenCL testing.`,
			},
			{
				Name: "graphics_video_platformdecoding",
				Desc: `Indicates that this test is exercising platform video decoding abilities.`,
			},
			{
				Name: "graphics_video_av1",
				Desc: `Indicates that this test is exercising av1 codec.`,
			},
			{
				Name: "graphics_video_chromestackdecoding",
				Desc: `Indicates that this test is part of Chrome Stack Decoding video.`,
			},
			{
				Name: "graphics_video_decodeaccel",
				Desc: `Indicates that this test is part of decode accel video.`,
			},
			{
				Name: "graphics_video_h264",
				Desc: `Indicates that this test is exercising h264 codec.`,
			},
			{
				Name: "graphics_video_hevc",
				Desc: `Indicates that this test is exercising hevc codec.`,
			},
			{
				Name: "graphics_video_vp8",
				Desc: `Indicates that this test is exercising vp8 codec.`,
			},
			{
				Name: "graphics_video_vp9",
				Desc: `Indicates that this test is exercising vp9 codec.`,
			},
		},
	},
	{
		Name:     "stress",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of stress tests.`,
	},
	{
		Name:     "arc-data-collector",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of ARC tests to be run in Android PFQ and collect data for specific Android build.`,
	},
	{
		Name:     "arc-functional",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of ARC Functional tests.`,
	},
	{
		Name:     "mtp",
		Contacts: []string{"arc-engprod@google.com"},
		Desc:     `A group of tests that run on DUTs with Android phones connected and verify MTP(Media Transfer Protocol).`,
	},
	{
		Name:     "appcompat",
		Contacts: []string{"chromeos-engprod@google.com", "cros-appcompat-test-team@google.com"},
		Desc:     `A group of ARC app compatibility tests.`,
		Subattrs: []*attr{
			{
				Name: "appcompat_release",
				Desc: `A group of ARC app compatibility tests for release testing.`,
			},
			{
				Name: "appcompat_smoke",
				Desc: `A group of ARC app compatibility tests for smoke testing.`,
			},
		},
	},
	{
		Name:     "arc-data-snapshot",
		Contacts: []string{"pbond@google.com", "arc-commercial@google.com"},
		Desc:     `A group of ARC data snapshot tests that run on DUTs.`,
	},
	{
		Name:     "camerabox",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `The group of camera tests to be run with Camerabox fixture.`,
		Subattrs: []*attr{
			{
				Name: "camerabox_facing_front",
				Desc: `Tests front camera functionalities using Camerabox front facing fixture.`,
			},
			{
				Name: "camerabox_facing_back",
				Desc: `Tests back camera functionalities using Camerabox back facing fixture.`,
			},
		},
	},
	{
		Name:     "camera-libcamera",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for libcamera build.`,
	},
	{
		Name:     "camera-postsubmit",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for postsubmit runs.`,
	},
	{
		Name:     "camera-usb-qual",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for USB camera qualification.`,
	},
	{
		Name:     "cuj",
		Contacts: []string{"chromeos-perfmetrics-eng@google.com"},
		Desc:     `A group of CUJ tests that run regularly for the Performance Metrics team.`,
	},
	{
		Name:     "drivefs-cq",
		Contacts: []string{"chromeos-files-syd@google.com"},
		Desc:     `The group of tests to be run in CQ for DriveFS functionality.`,
	},
	{
		Name:     "enrollment",
		Contacts: []string{"vsavu@google.com", "chromeos-commercial-remote-management@google.com"},
		Desc:     `A group of tests performing enrollment and will clobber the stateful partition.`,
	},
	{
		Name:     "dpanel-end2end",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DPanel/DMServer team.`,
	},
	{
		Name:     "input-tools",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     `A group of essential inputs IME and Virtual Keyboard tests.`,
	},
	{
		Name:     "input-tools-upstream",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     `A group of essential inputs IME and Virtual Keyboard tests running in google3.`,
	},
	{
		Name:     "firmware",
		Contacts: []string{"chromeos-engprod@google.com", "cros-fw-engprod@google.com"},
		Desc:     `A group of tests for firmware (AP, EC, GSC)`,
		Subattrs: []*attr{
			{
				Name: "firmware_bios",
				Desc: `A group of tests that test the AP firmware. Equivalent to autotest suite:faft_bios & suite:faft_bios_ro_qual & suite:faft_bios_rw_qual.`,
			},
			{
				Name: "firmware_ccd",
				Desc: `Indicates a test which requires a servo with CCD. I.e. A servo_v4 or equivalent.`,
			},
			{
				Name: "firmware_cr50",
				Desc: `Indicates that this is a test of the Google Security Chip firmware (Cr50).`,
			},
			{
				Name: "firmware_ec",
				Desc: `A group of tests that test the EC firmware. Equivalent to autotest suite:faft_ec & suite:faft_ec_fw_qual.`,
			},
			{
				Name: "firmware_experimental",
				Desc: `Firmware tests that might break the DUTs in the lab.`,
			},
			{
				Name: "firmware_slow",
				Desc: `A group of tests that takes a very long time to run.`,
			},
			{
				Name: "firmware_smoke",
				Desc: `A group of tests that exercise the basic firmware testing libraries. Equivalent to autotest suite:faft_smoke.`,
			},
			{
				Name: "firmware_unstable",
				Desc: `Firmware tests that are not yet stabilized, but won't break DUTs.`,
			},
			{
				Name: "firmware_usb",
				Desc: `Indicates a test which requires a working USB stick attached to the servo.`,
			},
			{
				Name: "firmware_bringup",
				Desc: `Indicates a test is safe to run on a board that doesn't boot to AP. Pass --var noSSH=true also.`,
			},
			{
				Name: "firmware_level1",
				Desc: `A subset of firmware_bios that is expected to pass before the AP firmware is finished.`,
			},
			{
				Name: "firmware_level2",
				Desc: `A subset of firmware_bios that is expected to pass after firmware_level1.`,
			},
		},
	},
	{
		Name:     "flashrom",
		Contacts: []string{"chromeos-platform-syd@google.com"},
		Desc:     `A group of Flashrom destructive tests.`,
	},
	{
		Name:     "hwsec_destructive_crosbolt",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc: `A group of HWSec destructive performance tests that wipe and recreate encstateful in the tests,
and run regularly in the crosbolt_perf_* suites.

Tests in this group are not used for build verification.`,
		Subattrs: []*attr{
			{
				Name: "hwsec_destructive_crosbolt_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
		},
	},
	{
		Name:     "hwsec_destructive_func",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of HWSec destructive tests that wipe and recreate encstateful in the tests.`,
	},
	{
		Name:     "labqual",
		Contacts: []string{"stagenut@google.com", "teravest@google.com"},
		Desc:     `A group of tests that must pass reliably prior to lab deployments.`,
	},
	{
		Name:     "omaha",
		Contacts: []string{"vsavu@google.com", "chromeos-commercial-remote-management@google.com"},
		Desc:     `A group of tests verifying the current state of Omaha.`,
	},
	{
		Name:     "rapid-ime-decoder",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     `A group of tests to validate libIMEdecoder.so releases.`,
	},
	{
		Name:     "runtime_probe",
		Contacts: []string{"chromeos-runtime-probe@google.com"},
		Desc: `A group of tests that tests the functionality of runtime probe.

The group of tests compare results probed by |runtime_probe| and corresponding information
in cros-labels decoded from HWID string.  These tests mainly check if runtime-probe works
as expected (in terms of D-Bus connection, probe function, and probe result).  For
short-term plan, only autotest can invoke these tests with host_info information.  That's
why we add this group and run tests of this group in a control file at
third_party/autotest/files/server/site_tests/tast/control.runtime-probe
`,
	},
	{
		Name:     "wificell",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     `The group of WiFi tests to be run with Wificell fixture.`,
		Subattrs: []*attr{
			{
				Name: "wificell_func",
				Desc: `Tests basic WiFi functionalities using Wificell fixture nightly.`,
			},
			{
				Name: "wificell_func_ax",
				Desc: `Tests basic WiFi AX functionalities using Wificell fixture nightly.`,
			},
			{
				Name: "wificell_suspend",
				Desc: `Tests basic WiFi behavior related to suspend/resume.`,
			},
			{
				Name: "wificell_cq",
				Desc: `Similar to wificell_func, but triggered by CLs that touch specific code paths.`,
			},
			{
				Name: "wificell_perf",
				Desc: `Measures WiFi performance using Wificell fixture nightly.`,
			},
			{
				Name: "wificell_stress",
				Desc: `Stress tests WiFi functionalities using Wificell fixture.`,
			},
			{
				Name: "wificell_mtbf",
				Desc: `Measure Mean Time Between Failures (MTBF) using Wificell fixture.`,
			},
			{
				Name: "wificell_unstable",
				Desc: `Indicates that this test is yet to be verified as stable.`,
			},
			{
				Name: "wificell_dut_validation",
				Desc: `Group of tests to be run by lab team to validate AP, PCAP, BT-Peers & DUT during deployment.`,
			},
		},
	},
	{
		Name:     "wificell_roam",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     `The group of WiFi roaming tests to be run with Grover fixture.`,
		Subattrs: []*attr{
			{
				Name: "wificell_roam_func",
				Desc: `Tests basic WiFi roaming functionalities using Grover fixture.`,
			},
			{
				Name: "wificell_roam_perf",
				Desc: `Measures WiFi performance using Grover fixture.`,
			},
		},
	},
	{
		Name:     "cellular",
		Contacts: []string{"chromeos-cellular-team@google.com"},
		Desc:     `The group of Cellular tests to be run on hardware with a builtin Cellular modem and SIM card.`,
		Subattrs: []*attr{
			{
				Name: "cellular_unstable",
				Desc: `Identifies Cellular tests that are unstable. Used to skip tests running on stable suites and/or the CQ.`,
			},
			{
				Name: "cellular_cq",
				Desc: `Identifies Cellular tests for the cellular commit queue suite.`,
			},
			{
				Name: "cellular_ota_avl",
				Desc: `Identifies Cellular ota tests for the cellular avl qual.`,
			},
			{
				Name: "cellular_sim_active",
				Desc: `Identifies Cellular tests that need an active sim.`,
			},
			{
				Name: "cellular_sim_dual_active",
				Desc: `Identifies Cellular tests that need active sim's on two slots.`,
			},
			{
				Name: "cellular_sim_roaming",
				Desc: `Identifies Cellular tests that need a roaming sim.`,
			},
			{
				Name: "cellular_sim_prod_esim",
				Desc: `Identifies Cellular tests that need an esim with a prod CI.`,
			},
			{
				Name: "cellular_sim_test_esim",
				Desc: `Identifies Cellular tests that need an esim with a test CI.`,
			},
			{
				Name: "cellular_modem_fw",
				Desc: `Identifies modem firmware tests that are run with less frequency.`,
			},
			{
				Name: "cellular_amari_callbox",
				Desc: `Identifies tests that run on DUTs connected to the Amari callbox.`,
			},
		},
	},
	{
		Name:     "meta",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `A group of functional tests of the Tast framework itself.

Meta tests should be a subset of mainline critical tests.
`,
	},
	{
		Name:     "fingerprint-cq",
		Contacts: []string{"chromeos-fingerprint@google.com"},
		Desc:     `The group of tests to be run in CQ for integrated fingerprint functionality.`,
	},
	{
		Name:     "fingerprint-mcu",
		Contacts: []string{"chromeos-fingerprint@google.com"},
		Desc:     `The group of tests to be run on a standalone Fingerprint MCU board.`,
		Subattrs: []*attr{
			{
				Name: "fingerprint-mcu_dragonclaw",
				Desc: `Tests to be run on Dragonclaw board (a standalone MCU board, not a Chrome OS board).`,
			},
			{
				Name: "fingerprint-mcu_icetower",
				Desc: `Tests to be run on Icetower board (a standalone MCU board, not a Chrome OS board).`,
			},
		},
	},
	{
		Name:     "storage-qual",
		Contacts: []string{"chromeos-engprod-platform-syd@google.com"},
		Desc:     `A group of tests for internal and external storage qualification and testing.`,
	},
	{
		Name:     "syzcorpus",
		Contacts: []string{"zsm@google.com", "chromeos-kernel@google.com"},
		Desc:     `Regression tests comprising of Syzkaller reproducers to test the kernel.`,
	},
	{
		Name:     "syzkaller",
		Contacts: []string{"zsm@google.com", "chromeos-kernel@google.com"},
		Desc:     `A group of tests that utilize Syzkaller to fuzz the kernel.`,
	},
	{
		Name:     "nearby-share",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Nearby Share functionality.`,
	},
	{
		Name:     "nearby-share-arc",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Nearby Share functionality from ARC++.`,
	},
	{
		Name:     "nearby-share-cq",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests to be run in CQ for Nearby Share functionality.`,
	},
	{
		Name:     "nearby-share-dev",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Nearby Share functionality with the dev version of Android Nearby.`,
	},
	{
		Name:     "nearby-share-prod",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Nearby Share functionality with the production version of Android Nearby.`,
	},
	{
		Name:     "nearby-share-remote",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of remote tests that test Nearby Share functionality.`,
	},
	{
		Name:     "paper-io",
		Contacts: []string{"project-bolton@google.com"},
		Desc:     `A group of tests that test printing and scanning functionality.`,
		Subattrs: []*attr{
			{
				Name: "paper-io_printing",
				Desc: `Printing tests.`,
			},
			{
				Name: "paper-io_scanning",
				Desc: `Scanning tests.`,
			},
			{
				Name: "paper-io_mfp_printscan",
				Desc: `Scanning and printing tests on real MFPs.`,
			},
		},
	},
	{
		Name:     "parallels_mainline",
		Contacts: []string{"parallels-cros@google.com"},
		Desc: `Functional tests that must be run on devices licensed for Parallels
boot-up testing. Otherwise the same as group:mainline.`,
		Subattrs: []*attr{
			{
				Name: "informational",
				Desc: `Indicates that failures can be ignored.`,
			},
		},
	},
	{
		Name:     "parallels_crosbolt",
		Contacts: []string{"crosbolt-eng@google.com"},
		Desc: `Performance tests that must be run on devices licensed for
Parallels boot-up testing. Otherwise the same as group:crosbolt.`,
		Subattrs: []*attr{
			{
				Name: "parallels_crosbolt_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
			{
				Name: "parallels_crosbolt_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "parallels_crosbolt_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
		},
	},
	{
		Name:     "typec",
		Contacts: []string{"chromeos-power@google.com"},
		Desc:     `USB Type C functional tests.`,
		Subattrs: []*attr{
			{
				Name: "typec_lab",
				Desc: `Indicates that this test should be run in a dedicated Type C lab setup.`,
			},
			{
				Name: "typec_informational",
				Desc: `Indicates that failures can be ignored.`,
			},
		},
	},
	{
		Name:     "borealis",
		Contacts: []string{"chromeos-gaming@google.com"},
		Desc:     `Borealis related tests.`,
		Subattrs: []*attr{
			{
				Name: "borealis_cq",
				Desc: `Indicate this test should be scheduled on cq.`,
			},
			{
				Name: "borealis_perbuild",
				Desc: `Indicate this test should be scheduled per build.`,
			},
			{
				Name: "borealis_nightly",
				Desc: `Indicate this test should be scheduled per day.`,
			},
			{
				Name: "borealis_weekly",
				Desc: `Indicate this test should be scheduled per week.`,
			},
			{
				Name: "informational",
				Desc: `Indicates that failures can be ignored.`,
			},
		},
	},
	{
		Name:     "cross-device",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Cross Device functionality between CrOS and Android devices.`,
	},
	{
		Name:     "hps",
		Contacts: []string{"chromeos-hps-swe@google.com"},
		Desc:     `HPS related tests.`,
		Subattrs: []*attr{
			{
				Name: "hps_perbuild",
				Desc: `Indicate this test should be scheduled per build.`,
			},
			{
				Name: "hps_devboard_p2_sweetberry",
				Desc: `Indicate this test depends on the HPS Devboard and Sweetberry to be connected.`,
			},
		},
	},
	{
		Name:     "wilco_bve",
		Contacts: []string{"lamzin@google.com", "cros-oem-services-team@google.com"},
		Desc:     `A group of Wilco tests that require servo type-A connected to a USB-A port that has a lightning bolt or a battery icon engraved into it.`,
	},
	{
		Name:     "wilco_bve_dock",
		Contacts: []string{"lamzin@google.com", "cros-oem-services-team@google.com"},
		Desc:     `A group of Wilco tests that require a solomon dock connected to the DUT.`,
	},
	{
		Name:     "autoupdate",
		Contacts: []string{"gabormagda@google.com", "cros-engprod-muc@google.com"},
		Desc:     `A group of tests that require the installation of a new OS image version.`,
	},
	{
		Name:     "asan",
		Contacts: []string{"cjdb@google.com", "chromeos-toolchain@google.com"},
		Desc:     `A group of tests for AddressSanitizer builds.`,
	},
	{
		Name:     "distributed_lab_qual",
		Contacts: []string{"chromeos-distributed-fleet-platform@google.com"},
		Desc:     `A group of test to qualify distributed lab components.`,
		Subattrs: []*attr{
			{
				Name: "distributed_lab_qual_faft",
				Desc: `Indicate firmware test for distributed lab.`,
			},
		},
	},
	{
		Name:     "shimless_rma",
		Contacts: []string{"chromeos-engprod-platform-syd@google.com"},
		Desc:     `shimless rma related tests.`,
		Subattrs: []*attr{
			{
				Name: "shimless_rma_experimental",
				Desc: `Shimless RMA tests that might break the DUTs in the lab.`,
			},
		},
	},
}

// validGroupMap is the name-keyed map of validGroups.
var validGroupMap = map[string]*group{}

func init() {
	// Initialize validGroupMap.
	for _, g := range validGroups {
		if _, ok := validGroupMap[g.Name]; ok {
			panic(fmt.Sprintf("Duplicated group definition %q found", g.Name))
		}
		validGroupMap[g.Name] = g
	}
}

const groupPrefix = "group:"

// checkKnownAttrs validate attrs against valid groups.
func checkKnownAttrs(attrs []string) error {
	const defPath = "chromiumos/tast/internal/testing/attr.go"

	var groups []*group
	for _, attr := range attrs {
		if isAutoAttr(attr) || !strings.HasPrefix(attr, groupPrefix) {
			continue
		}
		name := strings.TrimPrefix(attr, groupPrefix)
		g, ok := validGroupMap[name]
		if !ok {
			return fmt.Errorf("group %q is invalid; see %s for the full list of valid groups", name, defPath)
		}
		groups = append(groups, g)
	}

	for _, attr := range attrs {
		if isAutoAttr(attr) || strings.HasPrefix(attr, groupPrefix) {
			continue
		}
		// Allow the "disabled" attribute.
		// Note that manually-specified "disabled" attributes are checked on test instantiation
		// and prohibited. If we see the "disabled" attribute here, it is one internally added on
		// test instantiation for compatibility.
		// TODO(crbug.com/1005041): Remove this transitional handling.
		if attr == "disabled" {
			continue
		}
		found := false
	grouploop:
		for _, group := range groups {
			for _, subattr := range group.Subattrs {
				if attr == subattr.Name {
					found = true
					break grouploop
				}
			}
		}
		if !found {
			return fmt.Errorf("attribute %q is invalid in current groups; see %s for the full list of valid attributes", attr, defPath)
		}
	}

	return nil
}

// modifyAttrsForCompat modifies an attribute list for compatibility.
func modifyAttrsForCompat(attrs []string) []string {
	// If no "group:*" attribute is set, append the "disabled" attribute.
	// TODO(crbug.com/1005041): Remove this workaround once infra is updated to
	// not rely on the "disabled" attribute.
	hasGroup := false
	for _, a := range attrs {
		if strings.HasPrefix(a, groupPrefix) {
			hasGroup = true
			break
		}
	}

	if hasGroup {
		return attrs
	}
	return append(attrs, "disabled")
}
