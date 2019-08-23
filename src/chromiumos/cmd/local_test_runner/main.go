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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/bundle"
	"chromiumos/tast/crash"
	"chromiumos/tast/lsbrelease"
	"chromiumos/tast/runner"
	"chromiumos/tast/shutil"
)

func main() {
	args := runner.Args{
		RunTests: &runner.RunTestsArgs{
			BundleGlob: "/usr/local/libexec/tast/bundles/local/*",
			BundleArgs: bundle.RunTestsArgs{
				DataDir: "/usr/local/share/tast/data",
				TempDir: "/usr/local/tmp/tast/run_tmp",
			},
		},
	}
	cfg := runner.Config{
		Type:              runner.LocalRunner,
		KillStaleRunners:  true,
		SystemLogDir:      "/var/log",
		SystemLogExcludes: []string{"journal"}, // journald binary logs: https://crbug.com/931951
		JournaldSubdir:    "journal",           // destination for exported journald logs
		SystemInfoFunc:    writeSystemInfo,     // save additional system info at end of run
		SystemCrashDirs:   crash.DefaultDirs(),
		// The tast-use-flags package attempts to install this file to /etc,
		// but it gets diverted to /usr/local since it's installed for test images.
		USEFlagsFile: "/usr/local/etc/tast_use_flags.txt",
		SoftwareFeatureDefinitions: map[string]string{
			// This list is documented at docs/test_dependencies.md.
			// All USE flags referenced here must be listed in IUSE in the tast-use-flags ebuild.
			// The one exception is tast_vm, which is inserted by VM builders via -extrauseflags.
			"alt_syscall": `!"kernel-3_8" && !"kernel-3_10"`,
			"amd64":       "amd64",
			// There are three types of android dependency to differentiate between
			// ARC and ARCVM guest.
			// master-arc-dev and qt are under development and not stable to run Tast tests.
			"android":      `arc && !arcvm && !"android-container-master-arc-dev" && !"android-container-qt"`,
			"android_vm":   `arc && arcvm`,
			"android_both": `arc && !"android-container-master-arc-dev" && !"android-container-qt"`,
			// Run all ARC versions, including master-arc-dev and qt.
			"android_all":       `arc && !arcvm`,
			"android_all_both":  `arc`,
			"android_p":         `arc && "android-container-pi"`,
			"android_p_both":    `arc && ("android-container-pi" || "android-vm-pi")`,
			"arc_camera3":       `"arc-camera3"`,
			"aslr":              "!asan",                        // ASan instrumentation breaks ASLR
			"audio_play":        "internal_speaker && !tast_vm", // VMs and some boards don't have a speaker
			"audio_record":      "internal_mic && !tast_vm",     // VMs don't have a mic
			"biometrics_daemon": "biod",
			// TODO(b/73436929) Grunt cannot run 720p due to performance issue,
			// we should remove grunt after hardware encoding supported.
			// daisy variants' cameras don't support 1280x720.
			"camera_720p":             "!snow && !skate && !spring && !grunt",
			"chrome":                  "!chromeless_tty && !rialto",
			"chrome_internal":         "chrome_internal",
			"cros_config":             "unibuild",
			"cros_internal":           "internal",
			"cros_video_decoder":      "!disable_cros_video_decoder",
			"crosvm_gpu":              `"crosvm-gpu"`,
			"crossystem":              "!betty && !tast_vm", // VMs don't support few crossystem sub-commands: https://crbug.com/974615
			"cups":                    "cups",
			"diagnostics":             "diagnostics",
			"display_backlight":       "display_backlight",
			"dlc":                     "dlc_test",
			"drm_atomic":              "drm_atomic",
			"firewall":                "!moblab",                                // Moblab has relaxed iptables rules
			"google_virtual_keyboard": "chrome_internal && internal && !moblab", // doesn't work on Moblab: https://crbug.com/949912
			"gpu_sandboxing":          "!betty && !tast_vm",                     // no GPU sandboxing on VMs: https://crbug.com/914688
			"gsc":                     "cr50_onboard",
			"lock_core_pattern":       `"kernel-3_10" || "kernel-3_14" || "kernel-3_18"`,
			"memd":                    "memd",
			"ml_service":              "ml_service",
			"mosys":                   "!betty && !tast_vm",
			"no_android":              "!arc",
			"no_asan":                 "!asan",
			"no_msan":                 "!msan",
			"no_symlink_mount":        "!lxc", // boards using LXC set CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT=n
			"no_ubsan":                "!ubsan",
			"oci":                     "containers && !moblab", // run_oci doesn't work on Moblab: https://crbug.com/951691
			"qemu":                    "betty || tast_vm",
			"reboot":                  "!betty && !tast_vm",           // weird missing-runner-after-reboot bug: https://crbug.com/909955
			"screenshot":              "display_backlight && !rk3399", // screenshot command broken on RK3399: https://crbug.com/880597
			"selinux":                 "selinux",
			"selinux_current":         "selinux && !selinux_experimental",
			"selinux_experimental":    "selinux && selinux_experimental",
			"smartdim":                "smartdim",
			"storage_wearout_detect":  "storage_wearout_detect && !betty && !tast_vm", // Skip wearout checks for VMs and eMMC < 5.0
			"tablet_mode":             "touchview",
			"tpm":                     "!mocktpm && !tast_vm",
			"transparent_hugepage":    "transparent_hugepage",
			"usbguard":                "usbguard",
			"virtual_usb_printer":     "usbip",
			// Some VM builds actually can run nested VM with right host configuration.
			// But we haven't enable this feature on builders. For now, just disable
			// vm_host feature for VM builds. The kvm_transition flag indicates the
			// board may not work with VMs without a cold reboot b/134764918.
			"vm_host":    "kvm_host && !tast_vm && !kvm_transition",
			"vp9_sanity": "!rk3399", // RK3399 crashes on playing unsupported VP9 profile: https://crbug.com/971032
			"vulkan":     "vulkan",
			// nyan_kitty is skipped as its WiFi device is unresolvably flaky (crrev.com/c/944502),
			// exhibiting very similar symptoms to crbug.com/693724, b/65858242, b/36264732.
			"wifi":  "!betty && !tast_vm && !nyan_kitty",
			"wilco": "wilco",
		},
		// The autotest-capability package tries to install this to /etc but it's diverted to /usr/local.
		AutotestCapabilityDir: autocaps.DefaultCapabilityDir,
	}
	if kvs, err := lsbrelease.Load(); err == nil {
		if bp := kvs[lsbrelease.BuilderPath]; bp != "" {
			cfg.BuildArtifactsURL = "gs://chromeos-image-archive/" + bp + "/"
			cfg.PrivateBundleArchiveURL = cfg.BuildArtifactsURL + "tast_bundles.tar.bz2"
			cfg.PrivateBundlesStampPath = "/usr/local/share/tast/.private-bundles-downloaded"
		}
	}
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, &args, &cfg))
}

// writeSystemInfo writes additional system information from the DUT to files within dir.
func writeSystemInfo(ctx context.Context, dir string) error {
	runCmd := func(cmd *exec.Cmd, fn string) error {
		f, err := os.Create(filepath.Join(dir, fn))
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := fmt.Fprintf(f, "%q at end of testing:\n\n", shutil.EscapeSlice(cmd.Args)); err != nil {
			return err
		}
		cmd.Stdout = f
		cmd.Stderr = f
		return cmd.Run()
	}

	var errs []string
	for fn, cmd := range map[string]*exec.Cmd{
		"upstart_jobs.txt": exec.CommandContext(ctx, "initctl", "list"),
		"ps.txt":           exec.CommandContext(ctx, "ps", "auxwwf"),
		"du_stateful.txt":  exec.CommandContext(ctx, "du", "-m", "/mnt/stateful_partition"),
	} {
		if err := runCmd(cmd, fn); err != nil {
			errs = append(errs, fmt.Sprintf("failed running %q: %v", shutil.EscapeSlice(cmd.Args), err))
		}
	}

	// Also copy crash-related system info (e.g. /etc/lsb-release) to aid in debugging.
	// Having an easy way to see info about the system image (e.g. board name and version) is particularly useful.
	if err := crash.CopySystemInfo(dir); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ", "))
	}
	return nil
}
