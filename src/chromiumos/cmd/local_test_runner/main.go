// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the local_test_runner executable.
//
// local_test_runner is executed on-device by the tast command.
// It runs test bundles and reports the results back to tast.
// It is also used to query additional information about the DUT
// such as logs, crashes, and supported software features.
package main

import (
	"os"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/crash"
	"chromiumos/tast/runner"
)

func main() {
	args := runner.Args{
		BundleGlob:      "/usr/local/libexec/tast/bundles/local/*",
		DataDir:         "/usr/local/share/tast/data",
		TempDir:         "/usr/local/tmp/tast/run_tmp",
		SystemLogDir:    "/var/log",
		SystemCrashDirs: crash.DefaultDirs(),
		// The tast-use-flags package attempts to install this file to /etc,
		// but it gets diverted to /usr/local since it's installed for test images.
		USEFlagsFile: "/usr/local/etc/tast_use_flags.txt",
		SoftwareFeatureDefinitions: map[string]string{
			// This list is documented at docs/test_dependencies.md.
			// All USE flags referenced here must be listed in IUSE in the tast-use-flags ebuild.
			// The one exception is tast_vm, which is inserted by VM builders via -extrauseflags.
			"android":      "arc",
			"android_p":    `arc && "android-container-pi"`,
			"aslr":         "!asan",                                // ASan instrumentation breaks ASLR
			"audio_play":   "!betty && !tast_vm && !veyron_rialto", // VMs don't have audio hardware
			"audio_record": "internal_mic && !tast_vm",             // VMs don't have audio hardware
			// TODO(b/73436929) Grunt cannot run 720p due to performance issue,
			// we should remove grunt after hardware encoding supported.
			// daisy variants' cameras don't support 1280x720.
			"camera_720p":          "!snow && !skate && !spring && !grunt",
			"chrome":               "!chromeless_tty",
			"chrome_internal":      "chrome_internal",
			"chrome_login":         "!chromeless_tty && !rialto",
			"containers":           "containers",
			"cups":                 "cups",
			"display_backlight":    "display_backlight",
			"gpu_sandboxing":       "!betty && !tast_vm", // no GPU sandboxing on VMs: https://crbug.com/914688
			"memd":                 "memd",
			"ml_service":           "ml_service",
			"no_android":           "!arc",
			"no_symlink_mount":     "!lxc",                         // boards using LXC set CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT=n
			"reboot":               "!betty && !tast_vm",           // weird missing-runner-after-reboot bug: https://crbug.com/909955
			"screenshot":           "display_backlight && !rk3399", // screenshot command broken on RK3399: https://crbug.com/880597
			"selinux":              "selinux",
			"selinux_current":      "selinux && !selinux_experimental",
			"selinux_experimental": "selinux && selinux_experimental",
			"tablet_mode":          "touchview",
			"tpm":                  "!mocktpm && !tast_vm",
			// Some VM builds actually can run nested VM with right host configuration.
			// But we haven't enable this feature on builders. For now, just disable
			// vm_host feature for VM builds.
			"vm_host": "kvm_host && !tast_vm",
		},
		// The autotest-capability package tries to install this to /etc but it's diverted to /usr/local.
		AutotestCapabilityDir: autocaps.DefaultCapabilityDir,
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &args, runner.LocalRunner))
}
