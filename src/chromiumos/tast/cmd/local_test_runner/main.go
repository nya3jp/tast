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
	"chromiumos/tast/internal/crash"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/lsbrelease"
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
		Type:                  runner.LocalRunner,
		KillStaleRunners:      true,
		SystemLogDir:          "/var/log",
		SystemLogExcludes:     []string{"journal"}, // journald binary logs: https://crbug.com/931951
		UnifiedLogSubdir:      "unified",           // destination for exported unified system logs
		SystemInfoFunc:        writeSystemInfo,     // save additional system info at end of run
		SystemCrashDirs:       crash.DefaultDirs(),
		CleanupLogsPausedPath: "/var/lib/cleanup_logs_paused",
		// The tast-use-flags package attempts to install this file to /etc,
		// but it gets diverted to /usr/local since it's installed for test images.
		USEFlagsFile:   "/usr/local/etc/tast_use_flags.txt",
		LSBReleaseFile: lsbrelease.Path,
		SoftwareFeatureDefinitions: map[string]string{
			// This list is documented at docs/test_dependencies.md.
			// All USE flags referenced here must be listed in IUSE in the tast-use-flags ebuild.
			// The one exception is tast_vm, which is inserted by VM builders via -extrauseflags.
			"alt_syscall": `!"kernel-3_8" && !"kernel-3_10"`,
			"amd64":       "amd64",
			// ARC USE flags are defined here:
			// http://cs/chromeos_public/src/third_party/chromiumos-overlay/eclass/arc-build-constants.eclass
			"android_vm":         `arc && arcvm && !"android-vm-pi"`,
			"android_vm_r":       `arc && arcvm && "android-vm-rvc"`,
			"android_p":          `arc && "android-container-pi"`,
			"arc":                `arc`,
			"arc32":              `"cheets_user" || "cheets_userdebug"`,
			"arc64":              `"cheets_user_64" || "cheets_userdebug_64"`,
			"arc_camera1":        `"arc-camera1"`,
			"arc_camera3":        `"arc-camera3"`,
			"arc_launched_32bit": `"arc-launched-32bit-abi"`,
			"arc_launched_64bit": `"!arc-launched-32bit-abi"`,
			"arm":                `"arm" || "arm64"`,
			"aslr":               "!asan",                        // ASan instrumentation breaks ASLR
			"audio_play":         "internal_speaker && !tast_vm", // VMs and some boards don't have a speaker
			"audio_record":       "internal_mic && !tast_vm",     // VMs don't have a mic
			"biometrics_daemon":  "biod",
			"borealis_host":      "borealis_host",
			"breakpad":           "force_breakpad",
			// TODO(b/73436929) Grunt cannot run 720p due to performance issue,
			// we should remove grunt after hardware encoding supported.
			// daisy variants' cameras don't support 1280x720.
			"camera_720p":       "!snow && !skate && !spring && !grunt",
			"camera_legacy":     `!"arc-camera1" && !"arc-camera3"`,
			"cert_provision":    "cert_provision",
			"chrome":            "!chromeless_tty && !rialto",
			"chrome_internal":   "chrome_internal",
			"coresched":         "coresched",
			"crashpad":          "!force_breakpad",
			"cros_config":       "unibuild",
			"cros_internal":     "internal",
			"crosvm_gpu":        `"crosvm-gpu" && "virtio_gpu"`,
			"crosvm_no_gpu":     `!"crosvm-gpu" || !"virtio_gpu"`,
			"crossystem":        "!betty && !tast_vm", // VMs don't support few crossystem sub-commands: https://crbug.com/974615
			"cups":              "cups",
			"diagnostics":       "diagnostics && !betty && !tast_vm", // VMs do not have hardware to diagnose. https://crbug.com/1126619
			"display_backlight": "display_backlight",
			"dlc":               "dlc && dlc_test",
			"dptf":              "dptf",
			"device_crash":      `!("board:samus")`, // Samus devices do not reliably come back after kernel crashes. crbug.com/1045821
			"dmverity_stable":   `"kernel-3_8" || "kernel-3_10" || "kernel-3_14" || "kernel-3_18" || "kernel-4_4" || "kernel-4_14"`,
			"dmverity_unstable": `!("kernel-3_8" || "kernel-3_10" || "kernel-3_14" || "kernel-3_18" || "kernel-4_4" || "kernel-4_14")`,
			"drivefs":           "drivefs",
			"drm_atomic":        "drm_atomic",
			// asuka, banon, caroline, cave, celes, chell, cyan, edgar, kefka, reks, relm, sentry, terra, ultima, and wizpig have buggy EC firmware and cannot capture crash reports. b/172228823
			// drallion and sarien have do not support the "crash" EC command. crbug.com/1123716
			// guado, tidus, rikku, veyron_fievel, and veyron_tiger do not have EC firmware. crbug.com/1123716. TODO(crbug.com/1124554) Use an EC hardware dep for these rather than a software dep.
			// nocturne only sporadically captures EC panics. crbug.com/1135798
			// TODO(https://crbug.com/1122066): remove guado-cfm and rikku-cfm when they're no longer necessary
			"ec_crash":                `!(("board:asuka" || "board:banon" || "board:caroline" || "board:cave" || "board:celes" || "board:chell" || "board:cyan" || "board:edgar" || "board:kefka" || "board:reks" || "board:relm" || "board:sentry" || "board:terra" || "board:ultima" || "board:wizpig") || ("board:drallion" || "board:sarien") || ("board:guado" || "board:guado-cfm" || "board:tidus" || "board:rikku" || "board:rikku-cfm" || "board:veyron_fievel" || "board:veyron_tiger") || "board:nocturne")`,
			"encrypted_reboot_vault":  `!("kernel-3_8" || "kernel-3_10" || "kernel-3_14")`,
			"endorsement":             "!betty && !tast_vm", // VMs don't have valid endorsement certificate.
			"firewall":                "!moblab",            // Moblab has relaxed iptables rules
			"flashrom":                "!betty && !tast_vm",
			"google_virtual_keyboard": "chrome_internal && internal && !moblab", // doesn't work on Moblab: https://crbug.com/949912
			"gpu_sandboxing":          "!betty && !tast_vm",                     // no GPU sandboxing on VMs: https://crbug.com/914688
			"graphics_debugfs":        `!("kernel-3_8" || "kernel-3_10" || "kernel-3_14" || "kernel-3_18")`,
			"gsc":                     "cr50_onboard",
			"houdini":                 "houdini",
			"houdini64":               "houdini64",
			"hostap_hwsim":            "wifi_hostap_test",
			"igt":                     `("video_cards_amdgpu" || "video_cards_intel") && ("kernel-5_4" || "kernel-4_19" || "kernel-4_14")`,
			"iwlwifi_rescan":          "iwlwifi_rescan",
			"lacros":                  "!arm", // TODO(crbug.com/1144013): Expand this to include arm as well.
			"lock_core_pattern":       `"kernel-3_10" || "kernel-3_14" || "kernel-3_18"`,
			// QEMU has implemented memfd_create, but we haven't updated
			// to a release with the change (https://bugs.launchpad.net/qemu/+bug/1734792).
			// Remove "|| betty || tast_vm" from list when we upgrade.
			"memfd_create": `!("kernel-3_8" || "kernel-3_10" || "kernel-3_14" || betty || tast_vm)`,
			"memd":         "memd",
			// Only official builds are considered to have metrics consent.
			// See: ChromeCrashReporterClient::GetCollectStatsConsent()
			// Also metrics consent needs TPM (crbug.com/1035197).
			"metrics_consent":    "chrome_internal && !mocktpm && !tast_vm",
			"microcode":          "!betty && !tast_vm",
			"ml_benchmark":       "ml_benchmark_drivers",
			"ml_service":         "ml_service",
			"mosys":              "!betty && !tast_vm",
			"ndk_translation":    "ndk_translation",
			"ndk_translation64":  "ndk_translation64",
			"nnapi":              "nnapi",
			"no_android":         "!arc",
			"no_arm":             "!arm",
			"no_asan":            "!asan",
			"no_elm_hana_3_18":   `!((elm || hana) && "kernel-3_18")`, // board elm/hana with kernel-3.18 has issue performing WiFi scan: https://crbug.com/1015719
			"no_msan":            "!msan",
			"no_qemu":            "!betty && !tast_vm",
			"no_symlink_mount":   "!lxc", // boards using LXC set CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT=n
			"no_ubsan":           "!ubsan",
			"oci":                "containers && !moblab", // run_oci doesn't work on Moblab: https://crbug.com/951691
			"ocr":                "ocr",
			"plugin_vm":          "pita", // boards that can run Plugin VM.
			"proprietary_codecs": "chrome_internal || chrome_media",
			"pstore":             "!betty && !tast_vm", // These boards don't support pstore: https://crbug.com/971899
			"qemu":               "betty || tast_vm",
			"racc":               "racc",
			// weird missing-runner-after-reboot bug: https://crbug.com/909955
			// TODO(yich): This is a workaround to enable reboot flag on all boards.
			// We should disable this flag if the weird missing-runner-after-reboot bug still happening.
			// Or cleanup all reboot dependency in tast-tests.
			// Notice: The flag would be false when a board didn't have any attributes.
			"reboot":                 `"*"`,
			"screenshot":             "display_backlight && !rk3399", // screenshot command broken on RK3399: https://crbug.com/880597
			"selinux":                "selinux",
			"selinux_current":        "selinux && !selinux_experimental",
			"selinux_experimental":   "selinux && selinux_experimental",
			"shill-wifi":             "!moblab", // fizz-moblab disables the WiFi technology for Shill
			"smartdim":               "smartdim",
			"storage_wearout_detect": "storage_wearout_detect && !betty && !tast_vm", // Skip wearout checks for VMs and eMMC < 5.0
			"tablet_mode":            "touchview",
			"tpm":                    "!mocktpm",
			"tpm1":                   "!mocktpm && !tpm2", // Indicate tpm1.2 is available
			"tpm2":                   "!mocktpm && tpm2",  // Indicate tpm2 is available
			"transparent_hugepage":   "transparent_hugepage",
			"untrusted_vm":           `"kernel-4_19" || "kernel-5_4"`,
			"usbguard":               "usbguard",
			"vaapi":                  "vaapi",
			// As the direct video decoder is transitioned in there is the need
			// to run the legacy decoder to make sure it isn't broken and can be
			// rolled back to if the direct decoder is having problems.  On some
			// newer platforms there will not be a legacy decoder to run.
			"video_decoder_direct":           "!disable_cros_video_decoder",
			"video_decoder_legacy":           "disable_cros_video_decoder",
			"video_decoder_legacy_supported": `!("board:trogdor")`,
			// drm_atomic is a necessary but not sufficient condition to support
			// video_overlays; in practice, they tend to be enabled at the same time.
			// TODO(mcasas): query in advance for NV12 format DRM Plane support.
			"video_overlays":      "drm_atomic",
			"virtual_usb_printer": `!("kernel-3_8" || "kernel-3_10" || "kernel-3_14" || "kernel-4_4")`,
			// Some VM builds actually can run nested VM with right host configuration.
			// But we haven't enable this feature on builders. For now, just disable
			// vm_host feature for VM builds. The kvm_transition flag indicates the
			// board may not work with VMs without a cold reboot b/134764918.
			"vm_host":   "kvm_host && !tast_vm && !kvm_transition",
			"vp9_smoke": "!rk3399", // RK3399 crashes on playing unsupported VP9 profile: https://crbug.com/971032
			"vulkan":    "vulkan",
			"watchdog":  `watchdog`,
			// nyan_kitty is skipped as its WiFi device is unresolvably flaky (crrev.com/c/944502),
			// exhibiting very similar symptoms to crbug.com/693724, b/65858242, b/36264732.
			"wifi":        "!betty && !tast_vm && !nyan_kitty",
			"wilco":       "wilco",
			"wired_8021x": "wired_8021x",
			// TODO(crbug.com/1070299): Remove the below hard-coded devices and use
			// Intel WiFi dependency when wifi hardware dependencies are implemented.
			// TODO(crbug.com/1115620): remove "Elm" and "Hana" after unibuild migration
			// completed. Also, volteer is normally using Intel WiFi (HrP2), but the
			// devices in the lab are incorrectly equipped with Realtek RTL8822 chips.
			// This test should skip volteer devices until this is fixed (see b:171754540).
			// The list of boards with Intel WiFi chips is long, so instead of listing all
			// the boards that have Intel WiFi chips, skip the ones that don't have it.
			"intel_wifi_chip": `!("board:bob" || "board:elm" || "board:grunt" || "board:hana" || "board:jacuzzi" || "board:kevin" || "board:kukui" || "board:oak" || "board:scarlet" || "board:trogdor" || "board:volteer" || "board:veyron_fievel" || "board:veyron_mickey" || "board:veyron_tiger")`,
		},
		// The autotest-capability package tries to install this to /etc but it's diverted to /usr/local.
		AutotestCapabilityDir:   autocaps.DefaultCapabilityDir,
		PrivateBundlesStampPath: "/usr/local/share/tast/.private-bundles-downloaded",
	}
	if kvs, err := lsbrelease.Load(); err == nil {
		if bp := kvs[lsbrelease.BuilderPath]; bp != "" {
			cfg.DefaultBuildArtifactsURL = "gs://chromeos-image-archive/" + bp + "/"
			cfg.OSVersion = bp
		} else {
			// Sometimes CHROMEOS_RELEASE_BUILDER_PATH is not in /etc/lsb-release.
			// Make up the string in this case
			board := kvs[lsbrelease.Board]
			osVersion := kvs[lsbrelease.Version]
			milestone := kvs[lsbrelease.Milestone]
			buildType := kvs[lsbrelease.BuildType]
			cfg.OSVersion = fmt.Sprintf("%vR%v-%v (%v)", board, milestone, osVersion, buildType)
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
	cmds := map[string]*exec.Cmd{
		"upstart_jobs.txt": exec.CommandContext(ctx, "initctl", "list"),
		"ps.txt":           exec.CommandContext(ctx, "ps", "auxwwf"),
		"du_stateful.txt":  exec.CommandContext(ctx, "du", "-m", "/mnt/stateful_partition"),
		"mount.txt":        exec.CommandContext(ctx, "mount"),
		"hostname.txt":     exec.CommandContext(ctx, "hostname"),
		"uptime.txt":       exec.CommandContext(ctx, "uptime"),
		"losetup.txt":      exec.CommandContext(ctx, "losetup"),
		"df.txt":           exec.CommandContext(ctx, "df", "-mP"),
		"dmesg.txt":        exec.CommandContext(ctx, "dmesg"),
	}
	if _, err := os.Stat("/proc/bus/pci"); !os.IsNotExist(err) {
		cmds["lspci.txt"] = exec.CommandContext(ctx, "lspci", "-vvn")
	}

	for fn, cmd := range cmds {
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
