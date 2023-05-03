// Copyright 2019 The ChromiumOS Authors
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
post-submit testing in ChromeOS CI and Chromium CI are important places where
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
		Name:     "criticalstaging",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `Inidcates intent to run in critcial CQ & Release.

This group will be used to indicate a test is intended on going into "mainline"
critical testing. This group will be run on all boards/models; on ToT only.
Tests can only remain in this group long enough to gather signal (10 days),
after which the owner must promote them into mainline only, or back into
informational. If no owner action is taken after a 4 day grace period, they
will be moved into informational.
`,
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
				Desc: `Indicates that this test should run for every ChromeOS build.`,
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
			{
				Name: "crosbolt_arc_perf_qual",
				Desc: `Indicates that this test is used for ARC performance qualification.`,
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
				Desc: `Indicates that this test should run for every ChromeOS build.`,
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
				Desc: `Indicates that this test is running KMS igt-gpu-tools.`,
			},
			{
				Name: "graphics_chameleon_igt",
				Desc: `Indicates that this is a Chameleon test running igt-gpu-tools.`,
			},
			{
				Name: "graphics_opencl",
				Desc: `Indicates that this test is part of OpenCL testing.`,
			},
			{
				Name: "graphics_satlab_redrix_stress_b246324780",
				Desc: `Indicates that this test is running to support redrix b/246324780.`,
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
		Name:     "pvs",
		Contacts: []string{"chromeos-pvs-eng@google.com"},
		Desc:     `The group of pvs tests to be run regularly by the pvs team.`,
		Subattrs: []*attr{
			{
				Name: "pvs_perbuild",
				Desc: `Indicates that this test should run for every ChromeOS build.`,
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
		Name:     "arc-video",
		Contacts: []string{"chromeos-arc-video-eng@google.com"},
		Desc:     `A group of ARC Video tests.`,
	},
	{
		Name:     "mtp",
		Contacts: []string{"arc-engprod@google.com"},
		Desc:     `A group of tests that run on DUTs with Android phones connected and verify MTP(Media Transfer Protocol).`,
	},
	{
		Name:     "arc",
		Contacts: []string{"arc-engprod@google.com"},
		Desc:     `A group of tests that run ARC++ Functional Tests.`,
		Subattrs: []*attr{
			{
				Name: "arc_playstore",
				Desc: `A group of tests which tests playstore functionality on its nightly build.`,
			},
			{
				Name: "arc_core",
				Desc: `A group of tests which tests ARC Core functionality on its nightly build.`,
			},
			{
				Name: "arc_chromeos_vm",
				Desc: `A group of tests which run ARC functionality on ChromeOS VM nightly build.`,
			},
		},
	},
	{
		Name:     "appcompat",
		Contacts: []string{"arc-engprod@google.com", "cros-appcompat-test-team@google.com"},
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
			{
				Name: "appcompat_top_apps",
				Desc: `A group of ARC app compatibility tests for top apps testing.`,
			},
			{
				Name: "appcompat_default",
				Desc: `A group of ARC app compatibility tests for appcompat testing.`,
			},
		},
	},
	{
		Name:     "arcappgameperf",
		Contacts: []string{"arc-engprod@google.com"},
		Desc:     `A group of tests that run ARC++ Game performance tests.`,
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
		Name:     "cq-minimal",
		Contacts: []string{"bhthompson@google.com", "tast-owners@google.com"},
		Desc:     `A group of tests that verify minimum bisection/debug functionality`,
	},
	{
		Name:     "cq-medium",
		Contacts: []string{"dhaddock@google.com", "bhthompson@google.com", "tast-owners@google.com"},
		Desc:     `A group of tests providing medium level CQ coverage`,
	},
	{
		Name:     "cuj",
		Contacts: []string{"chromeos-perfmetrics-eng@google.com"},
		Desc:     `A group of CUJ tests that run regularly for the Performance Metrics team.`,
		Subattrs: []*attr{
			{
				Name: "cuj_experimental",
				Desc: `Experimental CUJ tests that only run on a selected subset of models.`,
			},
		},
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
		Name:     "dmserver-enrollment-daily",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DMServer enrollment.`,
	},
	{
		Name:     "dmserver-enrollment-live",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DMServer live enrollment.`,
	},
	{
		Name:     "dmserver-zteenrollment-daily",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DMServer ZTE enrollment.`,
	},
	{
		Name:     "dpanel-end2end",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DPanel/DMServer team.`,
	},
	{
		Name:     "enterprise-reporting",
		Contacts: []string{"albertojuarez@google.com", "cros-reporting-eng@google.com"},
		Desc:     `A group of tests for the commercial reporting/I&I team.`,
	},
	{
		Name:     "external-dependency",
		Contacts: []string{"chromeos-software-engprod@google.com", "shengjun@google.com"},
		Desc: `A group of tests that rely on external websites/apps/services. 
		Due to the dependencies to external resources, these test cases are more likely to break.
		Therefore, it is highly disrecommended to promote them into mainline CQ. 
		Please refer to go/cros-automation-1p3p for more details.`,
		Subattrs: []*attr{
			{
				Name: "external-dependency_exemption",
				Desc: `A group of tests with external dependencies that can run in mainline CQ.`,
			},
		},
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
		Contacts: []string{"chromeos-faft@google.com", "jbettis@chromium.org"},
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
			{
				Name: "firmware_level3",
				Desc: `A subset of firmware_bios that is expected to pass after firmware_level2.`,
			},
			{
				Name: "firmware_level4",
				Desc: `A subset of firmware_bios that is expected to pass after firmware_level3.`,
			},
			{
				Name: "firmware_detachable",
				Desc: `A set of non-destructive tests indented to run on detachables.`,
			},
			{
				Name: "firmware_trial",
				Desc: `Firmware tests that might leave the DUT in a state that will require flashing the AP/EC.`,
			},
			{
				Name: "firmware_stress",
				Desc: `Firmware tests which repeat the same scenario many times.`,
			},
		},
	},
	{
		Name:     "flashrom",
		Contacts: []string{"cros-flashrom-team@google.com"},
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
				Desc: `Indicates that this test should run for every ChromeOS build.`,
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
		Name:     "racc",
		Contacts: []string{"chromeos-runtime-probe@google.com"},
		Desc:     `A group of tests that validate the RACC-related functionality.`,
		Subattrs: []*attr{
			{
				Name: "racc_general",
				Desc: `Tests RACC functionality with RACC binaries installed.`,
			},
			{
				Name: "racc_config_installed",
				Desc: `Tests RACC functionality with probe payload installed.

The group of tests compare results probed by Runtime Probe and corresponding information
in cros-labels decoded from HWID string.  These tests mainly check if Runtime Probe works
as expected (in terms of D-Bus connection, probe function, and probe result).  For
short-term plan, only autotest can invoke these tests with host_info information.  That's
why we add this attribute and run tests of this attribute in a control file at
third_party/autotest/files/server/site_tests/tast/control.runtime-probe
`,
			},
		},
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
			{
				Name: "wificell_e2e",
				Desc: `Identifies wifi_chrome ui/e2e tests.`,
			},
			{
				Name: "wificell_e2e_unstable",
				Desc: `Identifies wifi_chrome ui/e2e tests that are unstable. Used to skip tests running on stable suites and/or the CQ.`,
			},
		},
	},
	{
		Name:     "wificell_cross_device",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     `The group of WiFi tests using nearbyshare fixture.`,
		Subattrs: []*attr{
			{
				Name: "wificell_cross_device_p2p",
				Desc: `Tests basic WiFi P2P functionalities using nearbyshare fixture.`,
			},
			{
				Name: "wificell_cross_device_tdls",
				Desc: `Tests basic WiFi TDLS functionalities using nearbyshare fixture.`,
			},
			{
				Name: "wificell_cross_device_sap",
				Desc: `Tests basic WiFi Soft AP functionalities using nearbyshare fixture.`,
			},
			{
				Name: "wificell_cross_device_unstable",
				Desc: `Indicates that this test is yet to be verified as stable.`,
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
				Name: "cellular_sim_pinlock",
				Desc: `Identifies Cellular tests that need sim with puk and pin codes.`,
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
				Desc: `Identifies tests that run on DUTs connected to an Amari callbox.`,
			},
			{
				Name: "cellular_cmw_callbox",
				Desc: `Identifies tests that run on DUTs connected to a CMW500 callbox.`,
			},
			{
				Name: "cellular_cmx_callbox",
				Desc: `Identifies tests that run on DUTs connected to a CMX500 callbox.`,
			},
			{
				Name: "cellular_callbox",
				Desc: `Identifies tests that run on DUTs connected to a callbox.`,
			},
			{
				Name: "cellular_e2e",
				Desc: `Identifies Cellular ui/e2e tests.`,
			},
			{
				Name: "cellular_sms",
				Desc: `Identifies SMS tests that can run on North America carriers AT&T, Verizon, T-Mobile.`,
			},
			{
				Name: "cellular_run_isolated",
				Desc: `Identifies tests which are scheduled seperately from other tests. These tests are called out by name in control files. Used to isolate stress tests or high priority tests`,
			},
			{
				Name: "cellular_carrier_att",
				Desc: `Identifies Cellular tests that need an AT&T active sim.`,
			},
			{
				Name: "cellular_carrier_verizon",
				Desc: `Identifies Cellular tests that need an Verizon active sim.`,
			},
			{
				Name: "cellular_carrier_tmobile",
				Desc: `Identifies Cellular tests that need an T-Mobile active sim.`,
			},
			{
				Name: "cellular_carrier_amarisoft",
				Desc: `Identifies Cellular tests that need an Amarisoft active sim.`,
			},
			{
				Name: "cellular_carrier_vodafone",
				Desc: `Identifies Cellular tests that need an Vodafone active sim.`,
			},
			{
				Name: "cellular_carrier_rakuten",
				Desc: `Identifies Cellular tests that need an Rakuten active sim.`,
			},
			{
				Name: "cellular_carrier_ee",
				Desc: `Identifies Cellular tests that need an EE active sim.`,
			},
			{
				Name: "cellular_carrier_kddi",
				Desc: `Identifies Cellular tests that need an KDDI active sim.`,
			},
			{
				Name: "cellular_carrier_docomo",
				Desc: `Identifies Cellular tests that need an Docomo active sim.`,
			},
			{
				Name: "cellular_carrier_softbank",
				Desc: `Identifies Cellular tests that need an Softbank active sim.`,
			},
			{
				Name: "cellular_carrier_fi",
				Desc: `Identifies Cellular tests that need an Google Fi active sim.`,
			},
			{
				Name: "cellular_carrier_local",
				Desc: `Identifies Cellular tests that need an active sim.`,
			},
		},
	},
	{
		Name:     "cellular_crosbolt",
		Contacts: []string{"chromeos-cellular-team@google.com"},
		Desc:     `The group of Cellular Performance tests to be run on hardware with a builtin Cellular modem and SIM card.`,
		Subattrs: []*attr{
			{
				Name: "cellular_crosbolt_perf_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "cellular_crosbolt_unstable",
				Desc: `Identifies Cellular tests that are unstable. Used to skip tests running on stable suites and/or the CQ.`,
			},
			{
				Name: "cellular_crosbolt_sim_active",
				Desc: `Identifies Cellular tests that need an active sim.`,
			},
			{
				Name: "cellular_crosbolt_carrier_att",
				Desc: `Identifies Cellular tests that need a AT&T active sim.`,
			},
			{
				Name: "cellular_crosbolt_carrier_verizon",
				Desc: `Identifies Cellular tests that need a Verizon active sim.`,
			},
			{
				Name: "cellular_crosbolt_carrier_tmobile",
				Desc: `Identifies Cellular tests that need a T-Mobile active sim.`,
			},
			{
				Name: "cellular_crosbolt_carrier_local",
				Desc: `Identifies Cellular tests that need a local active sim.`,
			},
		},
	},
	{
		Name:     "bluetooth",
		Contacts: []string{"cros-connectivity@google.com"},
		Desc:     "Identifies bluetooth tests.",
		Subattrs: []*attr{
			{
				Name: "bluetooth_sa",
				Desc: "Identifies stable bluetooth tests that only requires the DUT (not peer devices) to run except for stress, performance and MTBF tests. Previously known as bluetooth_standalone.",
			},
			{
				Name: "bluetooth_core",
				Desc: "Identifies stable bluetooth tests for bluetooth platform that requires a peer device.",
			},
			{
				Name: "bluetooth_floss",
				Desc: "Identifies stable bluetooth tests that are ported to run with the new floss stack. Eventually all tests in bluetooth_core and bluetooth_sa tests will be added to this pool and will be stabilised.",
			},
			{
				Name: "bluetooth_cross_device_fastpair",
				Desc: "Identifies stable Cross Device Fast Pair tests that require a peer device.",
			},
			{
				Name: "bluetooth_cross_device_fastpair_multidut",
				Desc: "Identifies stable Cross Device Fast Pair tests that require a peer device and 2 DUTs.",
			},
			{
				Name: "bluetooth_flaky",
				Desc: "Identifies bluetooth tests (bluetooth_sa and bluetooth_core) which are not stable yet. This is used to run new tests in the lab to detect any failures. Once the tests are stable (>95% pass rate), these tests are moved to bluetooth_sa or bluetooth_core suites",
			},
			{
				Name: "bluetooth_stress",
				Desc: "Identifies bluetooth stress tests.",
			},
			{
				Name: "bluetooth_sa_cq",
				Desc: "Identifies tests the same way as bluetooth_sa, but these tests are also ran as part of CQ for all changes.",
			},
			{
				Name: "bluetooth_core_cq",
				Desc: "Identifies tests the same way as bluetooth_core_cq, but these tests are also ran as part of the custom CQ for Bluetooth and WiFi changes.",
			},
			{
				Name: "bluetooth_floss_cq",
				Desc: "Identifies tests the same way as bluetooth_floss, but these tests are also ran as part of the custom CQ for Floss",
			},
			{
				Name: "bluetooth_cross_device_cq",
				Desc: "Identifies Cross Device Bluetooth tests that are also ran as part of the custom CQ for Cross Device.",
			},
			{
				Name: "bluetooth_wifi_coex",
				Desc: "Identifies bluetooth and wifi coexistence tests.",
			},
			{
				Name: "bluetooth_fw",
				Desc: "Identifies tests that can only break when a hardware/firmware change occurs. These tests test a feature/requirement that is implemented in hardware/firmware.",
			},
			{
				Name: "bluetooth_dep_feature",
				Desc: "Identifies tests for features that use Bluetooth. Example Nearby Share, Phone Hub etc.",
			},
			{
				Name: "bluetooth_perf",
				Desc: "Identifies performance tests for bluetooth.",
			},
			{
				Name: "bluetooth_longrun",
				Desc: "Identifies tests that take more than 5 minutes to run. This does not contain stress tests or MTBF tests. This allows for separate scheduling.",
			},
			{
				Name: "bluetooth_cuj",
				Desc: "Identifies tests for bluetooth that tests layers above the platform such as UI and any tests that implement a CUJ above platform layer.",
			},
			{
				Name: "bluetooth_manual",
				Desc: "Identifies semi-manual tests for bluetooth. Used as a logical grouping for these tests and are not scheduled in the lab.",
			},
			{
				Name: "bluetooth_avl",
				Desc: "Identifies AVL tests meant to be run by partners and are not scheduled in the lab.",
			},
			{
				Name: "bluetooth_mtbf",
				Desc: "Identifies MTBP tests for bluetooth. These are scheduled in a separate pool as to not use up all DUT capacity in the lab.",
			},
			{
				Name: "bluetooth_btpeers_1",
				Desc: "Identifies bluetooth tests that require at most 1 btpeer.",
			},
			{
				Name: "bluetooth_btpeers_2",
				Desc: "Identifies bluetooth tests that require at most 2 btpeers.",
			},
			{
				Name: "bluetooth_btpeers_3",
				Desc: "Identifies bluetooth tests that require at most 3 btpeers.",
			},
			{
				Name: "bluetooth_btpeers_4",
				Desc: "Identifies bluetooth tests that require at most 4 btpeers.",
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
				Desc: `Tests to be run on Dragonclaw board (a standalone MCU board, not a ChromeOS board).`,
			},
			{
				Name: "fingerprint-mcu_icetower",
				Desc: `Tests to be run on Icetower board (a standalone MCU board, not a ChromeOS board).`,
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
		Name:     "nearby-share-arc",
		Contacts: []string{"chromeos-sw-engprod@google.com", "arc-app-dev@google.com", "alanding@chromium.org"},
		Desc:     `A group of tests that test Nearby Share functionality from ARC++.`,
		Subattrs: []*attr{
			{
				Name: "nearby-share-arc_fusebox",
				Desc: `A group of tests that test Nearby Share functionality with FuseBox from ARC++.`,
			},
		},
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
				Desc: `Indicates that this test should run for every ChromeOS build.`,
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
		Name:     "wwcb",
		Contacts: []string{"cros-wwcb-automation@google.com"},
		Desc:     `WWCB end-to-end functional tests.`,
		Subattrs: []*attr{
			{
				Name: "wwcb_satlab",
				Desc: `Indicates that this test should be run in a dedicated wwcb setup.`,
			},
			{
				Name: "wwcb_informational",
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
		Subattrs: []*attr{
			{
				Name: "cross-device_crossdevice",
				Desc: `Temporary smoke test to mitigate cross device onboarding fixture failure.`,
			},
			{
				Name: "cross-device_cellular",
				Desc: `Cross Device tests that use a cellular connection on the Android phone.`,
			},
			{
				Name: "cross-device_cq",
				Desc: `Indicate this test should be scheduled on one of the Cross Device Commit Queues.`,
			},
			{
				Name: "cross-device_instanttether",
				Desc: `Instant Tether tests.`,
			},
			{
				Name: "cross-device_lacros",
				Desc: `Cross Device tests that use Lacros.`,
			},
			{
				Name: "cross-device_floss",
				Desc: `Cross Device tests with Floss enabled.`,
			},
			{
				Name: "cross-device_nearbyshare",
				Desc: `Nearby Share tests.`,
			},
			{
				Name: "cross-device_nearbyshare-dev",
				Desc: `A group of tests that test Nearby Share functionality with the dev version of Android Nearby.`,
			},
			{
				Name: "cross-device_nearbyshare-prod",
				Desc: `A group of tests that test Nearby Share functionality with the production version of Android Nearby.`,
			},
			{
				Name: "cross-device_phonehub",
				Desc: `Phone Hub tests.`,
			},
			{
				Name: "cross-device_smartlock",
				Desc: `Smart Lock tests.`,
			},
		},
	},
	{
		Name:     "cross-device-remote",
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Desc:     `A group of remote tests that test Cross Device functionality between two ChromeOS devices.`,
		Subattrs: []*attr{
			{
				Name: "cross-device-remote_cq",
				Desc: `Indicate this test should be scheduled on one of the Cross Device Commit Queues.`,
			},
			{
				Name: "cross-device-remote_nearbyshare",
				Desc: `Nearby Share tests.`,
			},
			{
				Name: "cross-device-remote_floss",
				Desc: `Cross Device remote tests with Floss enabled.`,
			},
		},
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
			{
				Name: "shimless_rma_normal",
				Desc: `Shimless RMA tests that can be executed in Skylab.`,
			},
			{
				Name: "shimless_rma_nodelocked",
				Desc: `Shimless RMA tests that require node-locked image.`,
			},
			{
				Name: "shimless_rma_calibration",
				Desc: `Shimless RMA tests for calibration.`,
			},
		},
	},
	{
		Name:     "ml_benchmark",
		Contacts: []string{"chromeos-platform-ml-accelerators@google.com"},
		Desc:     `MLBenchmark performance tests.`,
		Subattrs: []*attr{
			{
				Name: "ml_benchmark_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
		},
	},
	{
		Name:     "assistant_audiobox",
		Contacts: []string{"chromeos-sw-engprod@google.com", "assistive-eng@google.com"},
		Desc:     `A group of Assistant tests that have to be run in audiobox.`,
	},
	{
		Name:     "commercial_limited",
		Contacts: []string{"gabormagda@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of commercial tests that should use limited resources.`,
	},
	{
		Name:     "experimental",
		Contacts: []string{"chromeos-velocity@google.com"},
		Desc: `This is for housing Informational tests that are not going to be promoted to CQ any time soon (i.e. < 3months).
							 Default is to run the tests M/W/F only. Runs on same set of boards as group:mainline.
							 Motivation is to lower DUT usage for tests that don't need to run that frequently.`,
		Subattrs: []*attr{
			{
				Name: "experimental_nightly",
				Desc: `Runs once every evening.`,
			},
			{
				Name: "experimental_m_w_f",
				Desc: `Runs once on Monday, Wednesday, and Friday only.`,
			},
			{
				Name: "experimental_weekly",
				Desc: `Runs once a week only.`,
			},
		},
	},
	{
		Name:     "telemetry_extension_hw",
		Contacts: []string{"cros-oem-services-team@google.com"},
		Desc:     `A group of Telemetry Extension hardware dependant tests.`,
	},
	{
		Name:     "hwsec",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to hardware-backed security. (ie. TPM)`,
		Subattrs: []*attr{
			{
				Name: "hwsec_weekly",
				Desc: "Indicates that these tests should run weekly. Tests that aren't stable enough to be critical or is too expensive gets placed here",
			},
			{
				Name: "hwsec_nightly",
				Desc: "Indicates that these tests should run nightly. Tests that are too expensive for mainline, but still important enough to be checked more often than weekly, get placed here",
			},
		},
	},
	{
		Name:     "attestation",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the attestationd system daemon.`,
	},
	{
		Name:     "bootlockbox",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the bootlockboxd system daemon.`,
	},
	{
		Name:     "chaps",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the chapsd system daemon.`,
	},
	{
		Name:     "cryptohome",
		Contacts: []string{"cryptohome-core@google.com"},
		Desc:     `A group of tests related to the cryptohomed system daemon.`,
	},
	{
		Name:     "hwsec_infra",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the hwsec related low-level libraries & daemons.`,
	},
	{
		Name:     "tpm_manager",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the tpm_managerd system daemon.`,
	},
	{
		Name:     "u2fd",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the u2fd system daemon.`,
	},
	{
		Name:     "vtpm",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the vtpmd system daemon.`,
	},
	{
		Name:     "cros-tcp-grpc",
		Contacts: []string{"jonfan@google.com", "chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests that test Tast TCP based gRPC services.`,
	},
	{
		Name:     "sensors",
		Contacts: []string{"gwendal@google.com", "chromeos-sensors@google.com"},
		Desc:     `A group of tests that test sensor and sensor services.`,
	},
	{
		Name:     "network",
		Contacts: []string{"cros-networking@google.com"},
		Desc:     `A group of tests that test general network functions.`,
		Subattrs: []*attr{
			{
				Name: "network_e2e",
				Desc: `Identifies network ui/e2e tests.`,
			},
			{
				Name: "network_e2e_unstable",
				Desc: `Identifies network ui/e2e tests that are unstable. Used to skip tests running on stable suites and/or the CQ.`,
			},
			{
				Name: "network_platform",
				Desc: `Identifies stable platform network tests.`,
			},
			{
				Name: "network_platform_unstable",
				Desc: `Identifies unstable platform network tests.`,
			},
		},
	},
	{
		Name:     "intel-gating",
		Contacts: []string{"ambalavanan.m.m@intel.com", "intel-chrome-system-automation-team@intel.com"},
		Desc:     `A group of tests related to Intel that are gating for build validation.`,
	},
	{
		Name:     "hw_agnostic",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `Specifies tests that are capable of being run on VMs.

This does not imply tests can only run on VMs, but can be used by systems to find and schedule tests on VMs
from other existing groups (or use this group standalone).
`,
	},
	{
		Name:     "vdi_limited",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of VDI tests that should use run on selected devices to limit load on the infrastructure.`,
	},

	{
		Name:     "golden_tier",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `Suite will run assigned tests on devices that provide the most reliable results. More info go/cros-duop:proposal`,
	},
	{
		Name:     "medium_low_tier",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `Suite will run assigned tests on medium and low tier devices. More info go/cros-duop:proposal`,
	},
	{
		Name:     "hardware",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `Suite will run assigned tests on various hardware configurations. More info go/cros-duop:proposal`,
	},
	{
		Name:     "complementary",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `Suite will run assigned tests on devices that are not in golden_tier, medium_low_tier not hardware. More info go/cros-duop:proposal`,
	},
	{
		Name:     "crostini_slow",
		Contacts: []string{"clumptini@google.com"},
		Desc:     `A group of slow crostini tests that should run on dedicated shards only when relevant code is changed.`,
	},
	{
		Name:     "audio",
		Contacts: []string{"chromeos-audio-bugs@google.com", "chromeos-sw-engprod@google.com"},
		Desc:     `A group of audio tests that are out of audio.* but tracked with audio_cq.`,
	},
	{
		Name:     "audio_audiobox",
		Contacts: []string{"chromeos-audio-bugs@google.com", "chromeos-sw-engprod@google.com"},
		Desc:     `A group of audio tests that have to be run in audiobox.`,
	},
	{
		Name:     "unowned",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `Suite will run unowned tests at a minimul infra load.`,
	},
	{
		Name:     "video_conference",
		Contacts: []string{"cros-videoconferencing@google.com"},
		Desc:     `video conference related tests.`,
		Subattrs: []*attr{
			{
				Name: "video_conference_cq_critical",
				Desc: `Indicate this test should be scheduled in cq as critial test.`,
			},
			{
				Name: "video_conference_per_build",
				Desc: `Indicate this test should be scheduled per build.`,
			},
		},
	},
	{
		Name:     "camera_dependent",
		Contacts: []string{"chromeos-camera-eng@google.com", "chromeos-sw-engprod@google.com"},
		Desc:     `A group of tests depending on camera but out of camera.*.`,
	},
	{
		Name:     "power",
		Contacts: []string{"chromeos-platform-power@google.com", "chromeos-pvs-eng@google.com"},
		Desc:     `Tests measuring power.`,
		Subattrs: []*attr{
			{
				Name: "power_cpd",
				Desc: `Tests relying on CPD to measure power at the battery.`,
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
	const defPath = "go.chromium.org/tast/core/internal/testing/attr.go"

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
