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
		Name:     "cbx",
		Contacts: []string{"tast-core@google.com"},
		Desc:     `A group of tests related to cbx testing`,
		Subattrs: []*attr{
			{
				Name: "cbx_feature_enabled",
				Desc: "Indicates that these tests should run on devices that have cbx features",
			},
			{
				Name: "cbx_feature_disabled",
				Desc: "Indicates that these tests should run on devices that have no cbx features",
			},
			{
				Name: "cbx_critical",
				Desc: `Indicates that this test has been verified as P0 and stable.`,
			},
			{
				Name: "cbx_stable",
				Desc: `Indicates that this test has been verified as stable.`,
			},
			{
				Name: "cbx_unstable",
				Desc: `Indicates that this test is yet to be verified as stable.`,
			},
		},
	},
	{
		Name:     "chromeos_internal",
		Contacts: []string{"tast-core@google.com"},
		Desc:     `A group of tests that should not be run by public or general partners.`,
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
				Name: "crosbolt_arc_perf_memory_nightly",
				Desc: `Indicates that this test is a memory test for ARC performance qualification and should run nightly.`,
			},
			{
				Name: "crosbolt_arc_perf_qual",
				Desc: `Indicates that this test is used for ARC performance qualification.`,
			},
			{
				Name: "crosbolt_fsi_check",
				Desc: `Indicates that this test is used for FSI health check`,
			},
			{
				Name: "crosbolt_release_gates",
				Desc: `Indicates that this test is used for Release Gating.`,
			},
		},
	},
	{
		Name:     "flex_arcvm",
		Contacts: []string{"chromeos-flex-eng@google.com"},
		Desc:     `A group of ARCVM tests belonging to the ChromeOS Flex team.`,
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
				Name: "graphics_power",
				Desc: `Indicates that this test switches the power supply and discharges the battery.`,
			},
			{
				Name: "graphics_manual",
				Desc: `Indicates that this test is designed to run manually and not to run in the lab.`,
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
				Name: "graphics_deqp",
				Desc: `Indicates that this test is graphics deqp functional tests.`,
			},
			{
				Name: "graphics_cft",
				Desc: `Indicates that this test has been converted to directly run via suite_scheduler starlark file.`,
			},
			{
				Name: "graphics_bringup",
				Desc: `Indicates that this test should be run during devices bringup.`,
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
			{
				Name: "graphics_stress",
				Desc: `Indicates that this test is part of the graphics stress testing.`,
			},
			{
				Name: "graphics_tiled_satlab_perbuild",
				Desc: `Indicates that this test should run per-build on a satlab with tiled displays.`,
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
				Desc: `PVS tests that should run for every ChromeOS build.`,
			},
			{
				Name: "pvs_shop_daily",
				Desc: `SHoP tests that should run daily.`,
			},
		},
	},
	{
		Name:     "release-health",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `The group of tests to be considered release candidate quality signal.`,
		Subattrs: []*attr{
			{
				Name: "release-health_arc",
				Desc: "Release candidate ARC tests",
			},
			{
				Name: "release-health_arcappcompat",
				Desc: "Release candidate arcappcompat tests",
			},
			{
				Name: "release-health_audio",
				Desc: "Release candidate audio tests",
			},
			{
				Name: "release-health_autoupdate",
				Desc: "Release candidate autoupdate tests",
			},
			{
				Name: "release-health_borealis",
				Desc: "Release candidate borealis tests",
			},
			{
				Name: "release-health_bt",
				Desc: "Release candidate bluetooth tests",
			},
			{
				Name: "release-health_camera",
				Desc: "Release candidate camera tests",
			},
			{
				Name: "release-health_cellular",
				Desc: "Release candidate cellular tests",
			},
			{
				Name: "release-health_crashfeed",
				Desc: "Release candidate crash and feedback tests",
			},
			{
				Name: "release-health_cross_device",
				Desc: "Release candidate cross-device tests",
			},
			{
				Name: "release-health_enterprise",
				Desc: "Release candidate enterprise tests",
			},
			{
				Name: "release-health_essential_inputs",
				Desc: "Release candidate essential input tests",
			},
			{
				Name: "release-health_fingerprint",
				Desc: "Release candidate fingerprint tests",
			},
			{
				Name: "release-health_gfx",
				Desc: "Release candidate graphics tests",
			},
			{
				Name: "release-health_network",
				Desc: "Release candidate network tests",
			},
			{
				Name: "release-health_power",
				Desc: "Release candidate power tests",
			},
			{
				Name: "release-health_rlz",
				Desc: "Release candidate rlz tests",
			},
			{
				Name: "release-health_touch",
				Desc: "Release candidate touch tests",
			},
			{
				Name: "release-health_ui",
				Desc: "Release candidate UI tests",
			},
			{
				Name: "release-health_usb",
				Desc: "Release candidate usb-peripherals tests",
			},
			{
				Name: "release-health_vc",
				Desc: "Release candidate video conference tests",
			},
			{
				Name: "release-health_wifi",
				Desc: "Release candidate wifi tests",
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
		Contacts: []string{"chromeos-dev-engprod@google.com"},
		Desc:     `A group of tests that run on DUTs with Android phones connected and verify MTP(Media Transfer Protocol).`,
	},
	{
		Name:     "mtp_cq",
		Contacts: []string{"chromeos-dev-engprod@google.com"},
		Desc:     `A group of critical tests that run on DUTs with Android phones connected and verify MTP(Media Transfer Protocol).`,
	},
	{
		Name:     "arc",
		Contacts: []string{"chromeos-dev-engprod@google.com"},
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
		Contacts: []string{"chromeos-dev-engprod@google.com", "cros-appcompat-test-team@google.com"},
		Desc:     `A group of ARC app compatibility tests.`,
		Subattrs: []*attr{
			{
				Name: "appcompat_release",
				Desc: `A group of ARC app compatibility tests for release testing.`,
			},
			{
				Name: "appcompat_release_0",
				Desc: `Group 0 of ARC app compatibility tests for release testing.`,
			},
			{
				Name: "appcompat_release_1",
				Desc: `Group 1 of ARC app compatibility tests for release testing.`,
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
				Name: "appcompat_top_apps_0",
				Desc: `Group 0 of ARC app compatibility tests for top apps testing.`,
			},
			{
				Name: "appcompat_top_apps_1",
				Desc: `Group 1 of ARC app compatibility tests for top apps testing.`,
			},
			{
				Name: "appcompat_default",
				Desc: `A group of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_0",
				Desc: `Group 0 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_1",
				Desc: `Group 1 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_2",
				Desc: `Group 2 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_3",
				Desc: `Group 3 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_4",
				Desc: `Group 4 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_5",
				Desc: `Group 5 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_6",
				Desc: `Group 6 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_7",
				Desc: `Group 7 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_8",
				Desc: `Group 8 of ARC app compatibility tests for appcompat testing.`,
			},
			{
				Name: "appcompat_default_9",
				Desc: `Group 9 of ARC app compatibility tests for appcompat testing.`,
			},
		},
	},
	{
		Name:     "arcappgameperf",
		Contacts: []string{"chromeos-dev-engprod@google.com"},
		Desc:     `A group of tests that run ARC++ Game performance tests.`,
		Subattrs: []*attr{
			{
				Name: "arcappgameperf_0",
				Desc: `Group 0 of ARC++ Game performance tests.`,
			},
			{
				Name: "arcappgameperf_1",
				Desc: `Group 1 of ARC++ Game performance tests.`,
			},
			{
				Name: "arcappgameperf_2",
				Desc: `Group 2 of ARC++ Game performance tests.`,
			},
		},
	},
	{
		Name:     "arcappmediaperf",
		Contacts: []string{"chromeos-dev-engprod@google.com"},
		Desc:     `A group of tests that run ARC++ Media performance tests.`,
	},
	{
		Name:     "arc-data-snapshot",
		Contacts: []string{"pbond@google.com", "arc-commercial@google.com"},
		Desc:     `A group of ARC data snapshot tests that run on DUTs.`,
	},
	{
		Name:     "camera",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `The group of camera tests that can be run without special setup like camerabox.`,
		Subattrs: []*attr{
			{
				Name: "camera_cca",
				Desc: `Indicates the target is CCA.`,
			},
			{
				Name: "camera_service",
				Desc: `Indicates the target is camera service.`,
			},
			{
				Name: "camera_hal",
				Desc: `Indicates the target is HAL.`,
			},
			{
				Name: "camera_kernel",
				Desc: `Indicates the target is kernel.`,
			},
			{
				Name: "camera_functional",
				Desc: `Indicates the focus is functional verification.`,
			},
			{
				Name: "camera_pnp",
				Desc: `Indicates the focus is power and performance.`,
			},
			{
				Name: "camera_config",
				Desc: `Indicates the purpose is verification of configuration files.`,
			},
		},
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
		Name:     "camera-kernelnext",
		Contacts: []string{"chromeos-camera-kernel@google.com", "chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for verifying that kernel uprevs are working.`,
	},
	{
		Name:     "camera-libcamera",
		Contacts: []string{"chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for libcamera build.`,
	},
	{
		Name:     "camera-stability",
		Contacts: []string{"chromeos-camera-kernel@google.com", "chromeos-camera-eng@google.com"},
		Desc:     `A group of camera tests for verifying the stability of the camera modules.`,
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
		Contacts: []string{"cros-sw-perf@google.com"},
		Desc:     `A group of CUJ tests that run regularly for the Performance Metrics team.`,
		Subattrs: []*attr{
			{
				Name: "cuj_experimental",
				Desc: `Experimental CUJ tests that only run on a selected subset of models.`,
			},
			{
				Name: "cuj_weekly",
				Desc: `CUJ tests that run weekly`,
			},
			{
				Name: "cuj_loginperf",
				Desc: `LoginPerf.* that run regularly`,
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
		Name:     "data-leak-prevention-dmserver-enrollment-daily",
		Contacts: []string{"accorsi@google.com", "chromeos-dlp@google.com"},
		Desc:     `A group of Data Leak Prevention tests requiring DMServer enrollment.`,
	},
	{
		Name:     "dmserver-enrollment-daily",
		Contacts: []string{"rzakarian@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests for the DMServer enrollment.`,
	},
	{
		Name:     "powerwash-daily",
		Contacts: []string{"mohamedaomar@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests that perform powerwash.`,
	},
	{
		Name:     "meet-powerwash-daily",
		Contacts: []string{"joshuapius@google.com", "meet-devices-eng@google.com"},
		Desc:     `A group of tests that perform powerwash.`,
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
		Name:     "enterprise-reporting-daily",
		Contacts: []string{"albertojuarez@google.com", "cros-reporting-eng@google.com"},
		Desc:     `A group of tests for the commercial reporting/I&I team.`,
	},
	{
		Name:     "external-dependency",
		Contacts: []string{"chromeos-software-engprod@google.com"},
		Desc:     `A group of tests that rely on external dependencies such as services or apps`,
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
				Desc: `A group of tests that test the AP firmware.`,
			},
			{
				Name: "firmware_cr50",
				Desc: `Indicates that this is a test of the Google Security Chip firmware (Cr50).`,
			},
			{
				Name: "firmware_mp",
				Desc: `A group of tests targeted at mp_firmware_testing pool devices. These device run MP signed AP firmware and boot Test OS images via developer mode.`,
			},
			{
				Name: "firmware_ec",
				Desc: `A group of tests that test the EC firmware.`,
			},
			{
				Name: "firmware_enabled",
				Desc: `A group of tests that test the firmware. These tests must pass 100%% before exiting the Platform Enabled gate.`,
			},
			{
				Name: "firmware_meets_kpi",
				Desc: `A group of tests that test the firmware. These tests must pass 100%% before exiting the Meets KPI gate. This includes all "firmware_enabled" tests.`,
			},
			{
				Name: "firmware_stressed",
				Desc: `A group of tests that test the firmware. These tests must pass 100%% before exiting the Stressed gate. This includes all "firmware_enabled" and "firmware_meet_kpi" tests.`,
			},
			{
				Name: "firmware_experimental",
				Desc: `Firmware tests that might break the DUTs in the lab.`,
			},
			{
				Name: "firmware_pd",
				Desc: `A group of tests that test USB-C Power Delivery.`,
			},
			{
				Name: "firmware_pd_unstable",
				Desc: `Firmware PD tests that are not stabilized, but won't break DUTs.`,
			},
			{
				Name: "firmware_smoke",
				Desc: `A group of tests that exercise the basic firmware testing libraries.`,
			},
			{
				Name: "firmware_unstable",
				Desc: `Firmware tests that are not yet stabilized, but won't break DUTs.`,
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
				Name: "firmware_level5",
				Desc: `A subset of firmware_bios that is expected to pass after firmware_level4.`,
			},
			{
				Name: "firmware_trial",
				Desc: `Firmware tests that might leave the DUT in a state that will require flashing the AP/EC.`,
			},
			{
				Name: "firmware_stress",
				Desc: `Firmware tests which repeat the same scenario many times.`,
			},
			{
				Name: "firmware_ro",
				Desc: `Firmware tests which should only be run during an RO/RW qual, but not during a RW only qual.`,
			},
		},
	},
	{
		Name:     "flashrom",
		Contacts: []string{"cros-flashrom-team@google.com"},
		Desc:     `A group of Flashrom destructive tests.`,
	},
	{
		Name:     "gsc",
		Contacts: []string{"jettrink@google.com", "chromeos-faft@google.com"},
		Desc:     `A group of Google Security Chip tests -- typical performed on bare board.`,
		Subattrs: []*attr{
			{
				Name: "gsc_dt_ab",
				Desc: `A set of tests that run on a Dauntless Andreiboard with Hyperdebug connected.`,
			},
			{
				Name: "gsc_dt_shield",
				Desc: `A set of tests that run on a Dauntless Shield with Hyperdebug connected.`,
			},
			{
				Name: "gsc_ot_fpga_cw310",
				Desc: `A set of tests that run on an OpenTitan FPGACW310 with Hyperdebug connected.`,
			},
			{
				Name: "gsc_ot_shield",
				Desc: `A set of tests that run on an OpenTitan Teacup Shield with a NuvoTitan NT10 Z1 (engineering silicon) GSC and Hyperdebug connected.`,
			},
			{
				Name: "gsc_nt10_a1_shield",
				Desc: `A set of tests that run on an OpenTitan Teacup Shield with a NuvoTitan NT10 A1 GSC and Hyperdebug connected.`,
			},
			{
				Name: "gsc_nt11_a1_shield",
				Desc: `A set of tests that run on a Google Shield with a NuvoTitan NT11 A1 GSC and Hyperdebug connected.`,
			},
			{
				Name: "gsc_h1_shield",
				Desc: `A set of tests that run on a Haven Shield with Hyperdebug connected.`,
			},
			{
				Name: "gsc_he",
				Desc: `A set of tests that run on a emulated hardware via host process.`,
			},
			{
				Name: "gsc_image_ti50",
				Desc: `A set of tests that run on the normal ti50 FW image`,
			},
			{
				Name: "gsc_image_ti50a",
				Desc: `A set of tests that run on the ti50a FW image`,
			},
			{
				Name: "gsc_image_sta",
				Desc: `A set of tests that run on the system_test_auto FW image`,
			},
			{
				Name: "gsc_image_sta2",
				Desc: `A set of tests that run on the system_test_auto_2 FW image`,
			},
			{
				Name: "gsc_nightly",
				Desc: `A set of tests that should run nightly`,
			},
			{
				Name: "gsc_tcg",
				Desc: `A set of tests that belong to the TCG suite`,
			},
		},
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
		Name:     "labqual_informational",
		Contacts: []string{"peep-fleet-infra-sw@google.com", "gowriden@google.com"},
		Desc: `    A group of tests that will be ultimately included in the labqual group.
		           This separate group is for testing safely with backward compatibility.`,
	},
	{
		Name:     "labqual_stable",
		Contacts: []string{"peep-fleet-infra-sw@google.com", "gowriden@google.com"},
		Desc: `    A group of labqual tests that are verified to be stable(using suite scheduler) to be launched to external partners and internal stakeholders.
		           This separate group is for backward compatibility and not forcing to run the existing labqual group which contains optional tests.`,
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
third_party/autotest/files/server/site_tests/tast/control.racc-config-installed.
`,
			},
			{
				Name: "racc_encrypted_config_installed",
				Desc: `Tests RACC functionality with encrypted probe payload installed.

A same group as racc_config_installed, but only for devices with encrypted probe payload
installed.
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
				Name: "wificell_func_ax_e",
				Desc: `Tests basic WiFi 6GHz AX functionalities using Wificell fixture nightly.`,
			},
			{
				Name: "wificell_func_be",
				Desc: `Tests basic WiFi BE functionalities using Wificell fixture nightly.`,
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
			{
				Name: "wificell_reboot",
				Desc: `Using DUT reboot mechanism for the test.`,
			},
			{
				Name: "wificell_commercial",
				Desc: `Identifies wifi_commercial/enterprise tests.`,
			},
			{
				Name: "wificell_commercial_unstable",
				Desc: `Identifies wifi_commercial/enterprise tests that are unstable.`,
			},
		},
	},
	{
		Name:     "wificell_cross_device",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     `The group of WiFi tests running on multi-dut testbeds.`,
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
			{
				Name: "wificell_cross_device_multidut",
				Desc: `Tests basic WiFi mutidut connections to AP.`,
			},
			{
				Name: "wificell_cross_device_multidut_unstable",
				Desc: `Flaky tests that test basic WiFi mutidut connections to AP. Used to skip tests running on stable suites and/or the CQ.`,
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
		Name:     "wifi_router_feature",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     "Used with the wificell* test groups to denote features that all routers within the wificell testbed must support for the test. Each sub-attribute corresponds to a value in the chromiumos.test.lab.api.WifiRouterFeature Enum.",
		Subattrs: []*attr{
			{
				Name: "wifi_router_feature_ieee_802_11_a",
				Desc: "WiFi 1 (IEEE 802.11a) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_b",
				Desc: "WiFi 2 (IEEE 802.11b) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_g",
				Desc: "WiFi 3 (IEEE 802.11g) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_n",
				Desc: "WiFi 4 (IEEE 802.11n) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_ac",
				Desc: "WiFi 5 (IEEE 802.11ac) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_ax",
				Desc: "WiFi 6 (IEEE 802.11ax, 2.4GHz, 5GHz) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_ax_e",
				Desc: "WiFi 6E (IEEE 802.11ax, 6GHz) support.",
			},
			{
				Name: "wifi_router_feature_ieee_802_11_be",
				Desc: "WiFi 7 (IEEE 802.11be) support.",
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
				Name: "cellular_modem_verification",
				Desc: `Identifies modem tests used in CQ for manifest and DLC verification.`,
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
				Name: "cellular_suspend",
				Desc: `Identifies Cellular tests that will perform a suspend resume.`,
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
				Desc: `Identifies Cellular tests that need a Verizon active sim.`,
			},
			{
				Name: "cellular_carrier_tmobile",
				Desc: `Identifies Cellular tests that need a T-Mobile active sim.`,
			},
			{
				Name: "cellular_carrier_amarisoft",
				Desc: `Identifies Cellular tests that need an Amarisoft active sim.`,
			},
			{
				Name: "cellular_carrier_vodafone",
				Desc: `Identifies Cellular tests that need a Vodafone active sim.`,
			},
			{
				Name: "cellular_carrier_rakuten",
				Desc: `Identifies Cellular tests that need a Rakuten active sim.`,
			},
			{
				Name: "cellular_carrier_ee",
				Desc: `Identifies Cellular tests that need an EE active sim.`,
			},
			{
				Name: "cellular_carrier_kddi",
				Desc: `Identifies Cellular tests that need a KDDI active sim.`,
			},
			{
				Name: "cellular_carrier_docomo",
				Desc: `Identifies Cellular tests that need a Docomo active sim.`,
			},
			{
				Name: "cellular_carrier_softbank",
				Desc: `Identifies Cellular tests that need a Softbank active sim.`,
			},
			{
				Name: "cellular_carrier_fi",
				Desc: `Identifies Cellular tests that need a Google Fi active sim.`,
			},
			{
				Name: "cellular_carrier_bell",
				Desc: `Identifies Cellular tests that need a Bell active sim.`,
			},
			{
				Name: "cellular_carrier_roger",
				Desc: `Identifies Cellular tests that need a Rogers active sim.`,
			},
			{
				Name: "cellular_carrier_telus",
				Desc: `Identifies Cellular tests that need a Telus active sim.`,
			},
			{
				Name: "cellular_carrier_local",
				Desc: `Identifies Cellular tests that need an active sim.`,
			},
			{
				Name: "cellular_carrier_rak",
				Desc: `Identifies Cellular tests that need an active RAK sim.`,
			},
			{
				Name: "cellular_carrier_cbrs",
				Desc: `Identifies Cellular tests that need an active CBRS sim.`,
			},
			{
				Name: "cellular_carrier_linemo",
				Desc: `Identifies Cellular tests that need an active LINEMO sim.`,
			},
			{
				Name: "cellular_carrier_povo",
				Desc: `Identifies Cellular tests that need an active POVO sim.`,
			},
			{
				Name: "cellular_carrier_hanshin",
				Desc: `Identifies Cellular tests that need an active HANSHIN sim.`,
			},
			{
				Name: "cellular_carrier_agnostic",
				Desc: `Identifies Cellular tests whose functionality is independent of the carrier used.`,
			},
			{
				Name: "cellular_stress",
				Desc: `Identifies Cellular stress tests.`,
			},
			{
				Name: "cellular_power",
				Desc: `Identifies Cellular tests which measure and monitor power consumpiton with cellular connectivity.`,
			},
			{
				Name: "cellular_handover",
				Desc: `Identifies Cellular handover tests.`,
			},
			{
				Name: "cellular_carrier_dependent",
				Desc: `Identifies Cellular tests that should be only run on a single carrier`,
			},
			{
				Name: "cellular_hotspot",
				Desc: `Identifies Cellular hotspot tests.`,
			},
			{
				Name: "cellular_dut_check",
				Desc: `Identifies Cellular dut health tests.`,
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
		Name:     "uwb",
		Contacts: []string{"chromeos-cellular-team@google.com"},
		Desc:     `The group of Cellular Performance tests to be run on hardware with a builtin Cellular modem and SIM card.`,
		Subattrs: []*attr{
			{
				Name: "uwb_unstable",
				Desc: "Identifies UWB tests that are unstable. Used to skip tests running on stable suites and/or the CQ.",
			},
			{
				Name: "uwb_cros_peers_1",
				Desc: "Identifies uwb tests that require at most 1 CrOS peer.",
			},
			{
				Name: "uwb_cros_peers_2",
				Desc: "Identifies uwb tests that require at most 2 CrOS peers.",
			},
			{
				Name: "uwb_android_peers_1",
				Desc: "Identifies uwb tests that require at most 1 Android peer.",
			},
			{
				Name: "uwb_android_peers_2",
				Desc: "Identifies uwb tests that require at most 2 Android peers.",
			},
		},
	},
	{
		Name:     "bluetooth",
		Contacts: []string{"cros-device-enablement@google.com"},
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
				Name: "bluetooth_floss_flaky",
				Desc: "Identifies flaky bluetooth tests that are ported to run with the new floss stack. These tests will be moved to bluetooth_floss when stable.",
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
		},
	},
	{
		Name:     "meet",
		Contacts: []string{"rooms-engprod@google.com", "meet-devices-eng@google.com"},
		Desc:     `The group of tests to be run on a Meet CFM board.`,
		Subattrs: []*attr{
			{
				Name: "informational",
				Desc: `Used to gather information on test performance and confidence before being promoted to group:criticalstaging.`,
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
		Name:     "fingerprint-release",
		Contacts: []string{"chromeos-fingerprint@google.com"},
		Desc:     `The group of fingerprint tests to be included in e2e release testing.`,
	},
	{
		Name:     "fingerprint-informational",
		Contacts: []string{"chromeos-fingerprint@google.com"},
		Desc:     `The group of fingerprint tests to be used for informational testing.`,
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
			{
				Name: "fingerprint-mcu_quincy",
				Desc: `Tests to be run on Quincy board (a standalone MCU board, not a ChromeOS board).`,
			},
		},
	},
	{
		Name:     "storage-qual",
		Contacts: []string{"chromeos-storage@google.com"},
		Desc:     `A group of tests that are used for Storage HW validation.`,
		Subattrs: []*attr{
			{
				Name: "storage-qual_pdp_enabled",
				Desc: `Storage Enabled PDP gate tests.`,
			},
			{
				Name: "storage-qual_pdp_kpi",
				Desc: `Storage Meets KPI PDP gate tests.`,
			},
			{
				Name: "storage-qual_pdp_stress",
				Desc: `Storage Stress PDP gate tests.`,
			},
			{
				Name: "storage-qual_avl_v3",
				Desc: `Storage AVL v3 qualification tests.`,
			},
		},
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
		Contacts: []string{"chromeos-usb@google.com"},
		Desc:     `USB Type C functional tests.`,
		Subattrs: []*attr{
			{
				Name: "typec_compliance_ex350",
				Desc: `Indicates that this test should be run using an EX350 compliance tester.`,
			},
			{
				Name: "typec_lab",
				Desc: `Indicates that this test should be run in a dedicated Type C lab setup.`,
			},
			{
				Name: "typec_mcci",
				Desc: `Indicates that this test required a MCCI swtich.`,
			},
			{
				Name: "typec_dp_bringup",
				Desc: `Indicates that this test should be run for DisplayPort bringup validation`,
			},
			{
				Name: "typec_informational",
				Desc: `Indicates that failures can be ignored.`,
			},
			{
				Name: "typec_tbt3_bringup",
				Desc: `Indicates that this test should be run for Thunderbolt 3 bringup validation`,
			},
			{
				Name: "typec_tbt4_bringup",
				Desc: `Indicates that this test should be run for Thunderbolt 4 bringup validation`,
			},
			{
				Name: "typec_usb_bringup",
				Desc: `Indicates that this test should be run for USB-C bringup validation`,
			},
			{
				Name: "typec_unigraf274",
				Desc: `Indicates that this test should be run using an Unigraf UTC-274 tester.`,
			},
			{
				Name: "typec_manual",
				Desc: `Indicates that this test should be run only manually`,
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
		Name:     "pasit",
		Contacts: []string{"cros-wwcb-automation@google.com"},
		Desc:     `PASIT end-to-end functional tests.`,
		Subattrs: []*attr{
			{
				Name: "pasit_fast",
				Desc: `Indicates that this test without webcam should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_full",
				Desc: `Indicates that this test with webcam should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_storage",
				Desc: `Indicates that this test with exteranl storage should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_camera",
				Desc: `Indicates that this test with external camera should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_display",
				Desc: `Indicates that this test with external display should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_hid",
				Desc: `Indicates that this test with HIDs should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_pd",
				Desc: `Indicates that this test with USB power charging should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_fwupd",
				Desc: `Indicates that this test with external docks to test Fwupd should be run in a dedicated setup.`,
			},
			{
				Name: "pasit_informational",
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
				Name: "borealis_stress",
				Desc: `Indicate this is a stress test that should be scheduled on the Borealis satlab.`,
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
		Contacts: []string{"cros-oem-services-team@google.com"},
		Desc:     `A group of Wilco tests that require servo type-A connected to a USB-A port that has a lightning bolt or a battery icon engraved into it.`,
	},
	{
		Name:     "wilco_bve_dock",
		Contacts: []string{"cros-oem-services-team@google.com"},
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
			{
				Name: "shimless_rma_pretest",
				Desc: `Shimless RMA tests for devices not yet support Shimless RMA.`,
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
		Name:     "odml",
		Contacts: []string{"cros-odml-eng@google.com"},
		Desc:     `A group of tests related to the odmld daemon.`,
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
		Name:     "device_management",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of tests related to the device_management system daemon.`,
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
		Name:     "dsp",
		Contacts: []string{"peress@google.com", "chromeos-sensors@google.com"},
		Desc:     `A group of tests that are affected by using a DSP (like the ISH).`,
		Subattrs: []*attr{
			{
				Name: "dsp_ish",
				Desc: `Intel ISH specific tests.`,
			},
			{
				Name: "dsp_small",
				Desc: `Small tests that take < 10 minutes to run.`,
			},
			{
				Name: "dsp_medium",
				Desc: `Medium tests that take > 10 minutes but < 2 hours to run.`,
			},
			{
				Name: "dsp_large",
				Desc: `Large tests that take > 2 hours to run.`,
			},
		},
	},
	{
		Name:     "network",
		Contacts: []string{"cros-networking@google.com"},
		Desc:     `A group of tests that test general network functions.`,
		Subattrs: []*attr{
			{
				Name: "network_cq",
				Desc: `Identifies test that belong to network CQ and only gets triggered by CLs that touch specific code paths.`,
			},
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
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that are gating for build validation.`,
	},
	{
		Name:     "intel-nda",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that are No Devices Attached (NDA) for build validation.`,
	},
	{
		Name:     "intel-sleep",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that target the sleep(S0ix or S3) scenarios for build validation.`,
	},
	{
		Name:     "intel-convertible",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that are specific to convertible tablet mode DUTs for build validation.`,
	},
	{
		Name:     "intel-stability-bronze",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the stability of the builds on proto boards.`,
	},
	{
		Name:     "intel-stability-silver",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the stability of the builds on EVT boards.`,
	},
	{
		Name:     "intel-stability-gold",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the stability of the builds on DVT boards.`,
	},
	{
		Name:     "intel-reliability-bronze",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the reliability of the builds on proto boards.`,
	},
	{
		Name:     "intel-reliability-silver",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the reliability of the builds on EVT boards.`,
	},
	{
		Name:     "intel-reliability-gold",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that determines the reliability of the builds on DVT boards.`,
	},
	{
		Name:     "intel-stress",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that stresses the DUT for build validation.`,
	},
	{
		Name:     "intel-tbt3-dock",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT3 dock with 40G cable having USB 3.0 Pendrive, USB 3.2 Type-C Pendrive, 4K HDMI display and 3.5mm Jack peripherals for build validation.`,
	},
	{
		Name:     "intel-tbt4-dock",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT4/USB4 dock with 40G cable having USB 2.0 Pendrive, USB 3.0 Pendrive, 4K HDMI display and 3.5mm Jack peripherals for build validation.`,
	},
	{
		Name:     "intel-tbt3-dock-usb",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT3 dock with 40G cable having USB 2.0 Pendrive, USB 3.0 Pendrive, TBT3 BP SSD peripherals for build validation.`,
	},
	{
		Name:     "intel-tbt3-hdmi-dongle",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT to HDMI dongle connecting to a 4K HDMI display for build validation.`,
	},
	{
		Name:     "intel-tbt3-dp-dongle",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT to DP dongle connecting to a 4K DP display for build validation.`,
	},
	{
		Name:     "intel-tbt3-dock-tbt-display",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT3 dock with 40G cable having 4K TBT display for build validation.`,
	},
	{
		Name:     "intel-tbt4-dock-type-c-display",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT4/USB4 dock with 40G cable having Type-C display for build validation.`,
	},
	{
		Name:     "intel-type-c-display",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies Type-C display connected to Type-C port for build validation.`,
	},
	{
		Name:     "intel-tbt4-display",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT4/USB4 cable connecting to 4K TBT display for build validation.`,
	},
	{
		Name:     "intel-tbt3-dock-cbr",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT3 dock with active CBR cable for build validation.`,
	},
	{
		Name:     "intel-tbt4-dock-cbr",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT4/USB4 dock with active CBR cable for build validation.`,
	},
	{
		Name:     "intel-tbt3-bp-dock",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with TBT3 BP dock having USB 2.0 Pendrive, USB 3.0 Pendrive, 2K HDMI display and 3.5mm Jack build validation.`,
	},
	{
		Name:     "intel-cswitch-set1",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with Type-C to HDMI, Type-C to DP, Type-C Headset and Type-C to Type-A Hub with USB 3.0 Pendrive, USB 2.0 Pendrive and Type-A Headset peripherals for build validation.`,
	},
	{
		Name:     "intel-cswitch-set2",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses C-Switch with Type-C Pendrive, Type-C Keyboard and Type-C to Type-A Hub with Type-A Keyboard peripherals for build validation.`,
	},
	{
		Name:     "intel-dp",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies the DP display connected to the DUT via native DP port for build validation.`,
	},
	{
		Name:     "intel-dp-type-c",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies the DP display connected to the DUT via Type-C to DP dongle for build validation.`,
	},
	{
		Name:     "intel-hdmi",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies the HDMI display connected to the DUT via native HDMI port for build validation.`,
	},
	{
		Name:     "intel-hdmi-type-c",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies the HDMI display connected to the DUT via Type-C to HDMI dongle for build validation.`,
	},
	{
		Name:     "intel-bt",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that are specific to Bluetooth for build validation.`,
	},
	{
		Name:     "intel-flashing",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that flashes new builds on the DUT.`,
	},
	{
		Name:     "intel-jack",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel where 3.5mm Jack is connected to the DUT for build validation.`,
	},
	{
		Name: "intel_private",
		// TODO: b/295558201 - change to proper Intel owner later.
		Contacts: []string{"tast-owners@google.com"},
		Desc:     `Intel private tests.`,
		Subattrs: []*attr{
			{
				Name: "intel_private_unstable",
				Desc: `Unstable intel private tests.`,
			},
		},
	},
	{
		Name:     "intel-type-c-eth-dongle",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel where Ethernet is connected to the DUT via Type-C to Ethernet dongle for build validation.`,
	},
	{
		Name:     "intel-type-c-usb",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel where Type-C Pendrive is connected to the DUT for build validation.`,
	},
	{
		Name:     "intel-usb-cam",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel where Type-A USB camera is connected to the DUT for build validation.`,
	},
	{
		Name:     "intel-usb-set1",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses Type-A Hub with USB 3.0 Pendrive, USB 2.0 Pendrive and USB Speaker for build validation.`,
	},
	{
		Name:     "intel-usb-set2",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that uses Type-A Hub with USB Keyboard, USB Mouse and USB Headset for build validation.`,
	},
	{
		Name:     "intel-wlan",
		Contacts: []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		Desc:     `A group of tests related to Intel that verifies WLAN for build validation.`,
	},
	{
		Name:     "hw_agnostic",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `Specifies tests that are capable of being run on VMs.

This does not imply tests can only run on VMs, but can be used by systems to find and schedule tests on VMs
from other existing groups (or use this group standalone).
`,
		Subattrs: []*attr{
			{
				Name: "hw_agnostic_vm_stable",
				Desc: `Specifies tests that are stable on VMs.`,
			},
		},
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
		Name:     "audio_e2e_experimental",
		Contacts: []string{"chromeos-audio-bugs@google.com", "chromeos-sw-engprod@google.com", "crosep-intertech@google.com"},
		Desc:     `A group of audio end-to-end tests for experimental purposes.`,
		Subattrs: []*attr{
			{
				Name: "audio_e2e_experimental_audiobox",
				Desc: `Specifies tests that are running on audiobox`,
			},
			{
				Name: "audio_e2e_experimental_latency_toolkit",
				Desc: `Specifies tests that are running on audiobox with latency toolkit.`,
			},
			{
				Name: "audio_e2e_experimental_usb",
				Desc: `Specifies tests that are running on audio usb testbed.`,
			},
		},
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
			{
				Name: "power_regression",
				Desc: `Core tests designed to catch regressions in power consumption.`,
			},
			{
				Name: "power_daily",
				Desc: `Core tests designed to catch regressions in power consumption. Shorter tests, many models, ran daily.`,
			},
			{
				Name: "power_weekly",
				Desc: `Core tests designed to catch regressions in power consumption. Shorter tests, many models, ran weekly.`,
			},
			{
				Name: "power_daily_video_playback",
				Desc: `Core tests designed to catch regressions in power consumption. Only attached to tests in cros/power/video_playback.go. Ran daily.`,
			},
			{
				Name: "power_weekly_video_playback",
				Desc: `Core tests designed to catch regressions in power consumption. Only attached to tests in cros/power/video_playback.go. Ran weekly.`,
			},
			{
				Name: "power_daily_misc",
				Desc: `Miscellaneous tests designed to catch regressions and collect experimental data. Ran daily.`,
			},
			{
				Name: "power_weekly_misc",
				Desc: `Miscellaneous tests designed to catch regressions and collect experimental data. Ran weekly.`,
			},
			{
				Name: "power_regression_htl",
				Desc: `Core tests designed to catch regressions in power consumption in HTL lab.`,
			},
		},
	},
	{
		Name:     "healthd",
		Contacts: []string{"cros-tdm-tpe-eng@google.com"},
		Desc:     `A group of tests that validate the functionality of cros_healthd.`,
		Subattrs: []*attr{
			{
				Name: "healthd_perbuild",
				Desc: `Indicates that this test should run for every ChromeOS build.`,
			}, {
				Name: "healthd_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
		},
	},
	{
		Name:     "heartd",
		Contacts: []string{"cros-tdm-tpe-eng@google.com"},
		Desc:     `A group of tests that validate the functionality of heartd.`,
		Subattrs: []*attr{
			{
				Name: "heartd_perbuild",
				Desc: `Indicates that this test should run for every ChromeOS build.`,
			},
		},
	},
	{
		Name:     "launcher_search_quality_daily",
		Contacts: []string{"launcher-search-notify@google.com"},
		Desc:     "A group of tests for checking the pass ratio in different builds for launcher search.",
	},
	{
		Name:     "launcher_search_quality_per_build",
		Contacts: []string{"launcher-search-notify@google.com"},
		Desc:     "A group of tests for checking the pass ratio in different builds for launcher search.",
	},
	{
		Name:     "language_packs_hw_recognition_dlc_download_daily",
		Contacts: []string{"cros-borders-eng@google.com"},
		Desc:     "A group of tests for checking language packs handwriting recognition dlc downloading",
	},
	{
		Name:     "ddd_test_group",
		Contacts: []string{"chromeos-test-platform-team@google.com", "dbeckett@google.com"},
		Desc:     "A group of tests for checking the 3d expression resolver.",
	},
	{
		Name:     "privacyhub-golden",
		Contacts: []string{"chromeos-privacyhub@google.com", "zauri@google.com"},
		Desc:     "A group of tests for checking the Privacy Hub features on golden devices.",
	},
	{
		Name:     "lvm_migration",
		Contacts: []string{"chromeos-storage@google.com", "sarthakukreti@google.com"},
		Desc:     "A group of tests for checking the LVM migration feature.",
		Subattrs: []*attr{
			{
				Name: "lvm_migration_cryptohome",
				Desc: "Tests for validating cryptohome validity across migration.",
			},
		},
	},
	// DO NOT USE for test suites that are not owned by the ChromeOS Software - Developer pillar team.
	// This is subject to change in the future.
	// TODO(b/291014686) Review this setting to check if there is any issue.
	{
		Name:     "cros_dev_suite",
		Contacts: []string{"chromeos-dev-engprod@google.com"},
		Desc:     "A group used to divide large test suites for ChromeOS Developer (F.K.A ChromeOS Apps) focus area tests",
		Subattrs: []*attr{
			{
				Name: "cros_dev_suite_0",
				Desc: "Index 0 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_1",
				Desc: "Index 1 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_2",
				Desc: "Index 2 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_3",
				Desc: "Index 3 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_4",
				Desc: "Index 4 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_5",
				Desc: "Index 5 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_6",
				Desc: "Index 6 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_7",
				Desc: "Index 7 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_8",
				Desc: "Index 8 of the CrOS dev sub-suites",
			},
			{
				Name: "cros_dev_suite_9",
				Desc: "Index 9 of the CrOS dev sub-suites",
			},
		},
	},
	{
		Name:     "human_motion_robot",
		Contacts: []string{"chromeos-tango@google.com"},
		Desc:     "A group of tests that use the Human Motion Robot device",
		Subattrs: []*attr{
			{
				Name: "human_motion_robot_linearity",
				Desc: "A group of tests that focus on linearity",
			},
			{
				Name: "human_motion_robot_latency",
				Desc: "A group of tests that focus on measuring latency",
			},
		},
	},
	{
		Name:     "launcher_image_search_perbuild",
		Contacts: []string{"launcher-search-notify@google.com"},
		Desc:     "A group of launcher image search test cases for dedicated support boards.",
	},
	{
		Name:     "cr_oobe",
		Contacts: []string{"cros-device-enablement@google.com"},
		Desc:     "A group of tests for OOBE maintained by ChromeOS Device Enablement team",
		Subattrs: []*attr{
			{
				Name: "cr_oobe_chromebox_chromebase",
				Desc: "A group of oobe tests that need chromebox/chromebase",
			},
		},
	},
	{
		Name:     "floatingworkspace",
		Contacts: []string{"cros-commercial-productivity-eng@google.com"},
		Desc:     "A group of tests related to floating workspace.",
	},
	{
		Name:     "inputs_appcompat_arc_perbuild",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     "A group of inputs test on arc++ platform for most popular boards.",
	},
	{
		Name:     "inputs_orca_daily",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     "A group of inputs test for orca feature that runs daily.",
	},
	{
		Name:     "inputs_appcompat_citrix_perbuild",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     "A group of inputs test on citrix platform for most popular boards.",
	},
	{
		Name:     "crostini_cq",
		Contacts: []string{"clumptini@google.com"},
		Desc:     "A group of tests for Crostini non-apps tests that run in CQ.",
	},
	{
		Name:     "crostini_app_cq",
		Contacts: []string{"clumptini@google.com"},
		Desc:     "A group of tests for Crostini apps tests that run in CQ.",
	},
	{
		Name:     "bruschetta_cq",
		Contacts: []string{"clumptini@google.com"},
		Desc:     "A group of tests for bruschetta tests that run in CQ.",
	},
	{
		Name:     "tape-daily",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     "A group of tests that use accounts provided by TAPE.",
	},
	{
		Name:     "on_flex",
		Contacts: []string{"kamilszarek@google.com", "chromeos-ent-test@google.com"},
		Desc:     `A group of tests that run on Flex devices.`,
	},
	{
		Name:     "crospts",
		Contacts: []string{"cros-core-systems-perf@google.com"},
		Desc:     `A group of performance microbenchmark tests that run manually.`,
		Subattrs: []*attr{
			{
				Name: "crospts_x86",
				Desc: `CrosPTS tests for x86.`,
			},
			{
				Name: "crospts_arm64",
				Desc: `CrosPTS tests for arm64.`,
			},
		},
	},
	{
		Name:     "secagentd_bpf",
		Contacts: []string{"cros-enterprise-security@google.com"},
		Desc:     `A group of ChromeOS Enterprise XDR tests that use eBPF.`,
	},
	{
		Name:     "chrome_uprev_cbx",
		Contacts: []string{"chromeos-velocity@google.com", "alvinjia@google.com"},
		Desc:     `A group of tests that are running on cbx device in Chrome uprev CQ.`,
	},
	{
		Name:     "sw_gates_virt",
		Contacts: []string{"crosvm-core@google.com"},
		Desc:     `A group of tests that belong to the virtualization part of the Platform enablement SW gates.`,
		Subattrs: []*attr{
			{
				Name: "sw_gates_virt_enabled",
				Desc: `Enabled tests.`,
			},
			{
				Name: "sw_gates_virt_kpi",
				Desc: `Meets KPIs tests.`,
			},
			{
				Name: "sw_gates_virt_stress",
				Desc: `Stress tests.`,
			},
		},
	},
	{
		Name:     "inputs_appcompat_gworkspace_perbuild",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     "A group of inputs test for gworkspace",
	},
	{
		Name:     "video_conference_face_framing_per_build",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     "A group of tests for video conference on face farming device.",
	},
	{
		Name:     "fwupd",
		Contacts: []string{"chromeos-fwupd@google.com", "rishabhagr@google.com"},
		Desc:     "A group of fwupd related tests to run on builders",
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
