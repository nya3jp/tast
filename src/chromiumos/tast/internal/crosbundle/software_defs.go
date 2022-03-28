// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

// softwareFeatureDefs defines software features for Chrome OS test bundles.
var softwareFeatureDefs = map[string]string{
	// This list is documented at docs/test_dependencies.md.
	// All USE flags referenced here must be listed in IUSE in the tast-use-flags ebuild.
	// The one exception is tast_vm, which is inserted by VM builders via -extrauseflags.
	"amd64": "amd64",
	// ARC USE flags are defined here:
	// http://cs/chromeos_public/src/third_party/chromiumos-overlay/eclass/arc-build-constants.eclass
	// TODO(b/210917721): Re-enable ARCVM on manatee.
	"android_vm":   `arc && arcvm && !"android-vm-pi" && !manatee`,
	"android_vm_r": `arc && arcvm && "android-vm-rvc" && !manatee`,
	"android_p":    `arc && "android-container-pi"`,
	// TODO(b/210917721): Re-enable ARCVM on manatee.
	"arc":                `arc && !manatee`,
	"arc32":              `"cheets_user" || "cheets_userdebug"`,
	"arc64":              `"cheets_user_64" || "cheets_userdebug_64"`,
	"arc_camera1":        `"arc-camera1"`,
	"arc_camera3":        `"arc-camera3"`,
	"arc_launched_32bit": `"arc-launched-32bit-abi"`,
	"arc_launched_64bit": `"!arc-launched-32bit-abi"`,
	"arm":                `"arm" || "arm64"`,
	"aslr":               "!asan", // ASan instrumentation breaks ASLR
	"biometrics_daemon":  "biod",
	// TODO(b/225065082): Re-enable borealis on manatee
	"borealis_host": "borealis_host && !manatee",
	"breakpad":      "force_breakpad",
	// daisy variants' cameras don't support 1280x720.
	"camera_720p": "!snow && !skate && !spring",
	// Some boards might not support the camera/video/audio components required by the camera app.
	// TODO(b/185087278): Remove soraka-libcamera.
	"camera_app":            `!("board:volteer-kernelnext" || "board:soraka-libcamera")`,
	"camera_feature_hdrnet": "camera_feature_hdrnet",
	"camera_legacy":         `!"arc-camera1" && !"arc-camera3"`,
	"cert_provision":        "cert_provision",
	"chrome":                "!chromeless_tty && !rialto",
	"chrome_internal":       "chrome_internal",
	"chromeless":            "chromeless_tty || rialto",
	"coresched":             "coresched",
	// TODO(b/174888780) Remove kernel-4_4 once arm64 kernel reporting is fixed
	// TODO(b/174889440) Remove hana, elm, kevin, bob, scarlet.
	// Per b/175345642 veryon_fievel/veyron_tiger are safe but arm32 doesn't report anything in sysfs so just ignore these boards
	"cpu_vuln_sysfs": `!(("kernel-4_4" && ("arm" || "arm64")) || "board:bob" || "board:hana" || "board:elm" || "board:hana-kernelnext" || "board:elm-kernelnext" || "board:kevin" || "board:kevin-kernelnext" || "board:scarlet" || "board:scarlet-kernelnext" || "board:veyron_fievel" || "board:veyron_tiger")`,
	"cras":           "cras",
	"crashpad":       "!force_breakpad",
	"cros_internal":  "internal",
	"crosvm_gpu":     `"crosvm-gpu" && "virtio_gpu"`,
	"crosvm_no_gpu":  `!"crosvm-gpu" || !"virtio_gpu"`,
	// VMs don't support few crossystem sub-commands: https://crbug.com/974615
	"crossystem":        `!"betty" && !"tast_vm"`,
	"cups":              "cups",
	"diagnostics":       `"diagnostics" && !"betty" && !"tast_vm"`, // VMs do not have hardware to diagnose. https://crbug.com/1126619
	"dlc":               "dlc && dlc_test",
	"dptf":              "dptf",
	"device_crash":      `!("board:samus")`, // Samus devices do not reliably come back after kernel crashes. crbug.com/1045821
	"dmverity_stable":   `"kernel-3_18" || "kernel-4_4" || "kernel-4_14"`,
	"dmverity_unstable": `!("kernel-3_18" || "kernel-4_4" || "kernel-4_14")`,
	"drivefs":           "drivefs",
	"drm_atomic":        "drm_atomic",
	"drm_trace":         `!("kernel-3_18" || "kernel-4_4" || "kernel-4_14" || "kernel-4_19")`,
	// asuka, banon, caroline, cave, celes, chell, cyan, edgar, kefka, reks, relm, sentry, terra, ultima, and wizpig have buggy EC firmware and cannot capture crash reports. b/172228823
	// drallion and sarien have do not support the "crash" EC command. crbug.com/1123716
	// guado, tidus, rikku, veyron_fievel, and veyron_tiger do not have EC firmware. crbug.com/1123716. TODO(crbug.com/1124554) Use an EC hardware dep for these rather than a software dep.
	// nocturne only sporadically captures EC panics. crbug.com/1135798
	// TODO(https://crbug.com/1122066): remove guado-cfm and rikku-cfm when they're no longer necessary
	// TODO(b/201430283): Remove hatch, hatch-arc-r, nami-kernelnext, rammus, and sarien-kernelnext when bug is resolved.
	"ec_crash":                `!(("board:asuka" || "board:banon" || "board:caroline" || "board:caroline-kernelnext" || "board:caroline-userdebug" || "board:cave" || "board:celes" || "board:chell" || "board:cyan" || "board:edgar" || "board:kefka" || "board:reks" || "board:relm" || "board:sentry" || "board:terra" || "board:ultima" || "board:wizpig") || ("board:drallion" || "board:sarien") || ("board:guado" || "board:guado-cfm" || "board:tidus" || "board:rikku" || "board:rikku-cfm" || "board:veyron_fievel" || "board:veyron_tiger") || "board:nocturne" || "board:nocturne-kernelnext" || "board:hatch" || "board:hatch-arc-r" || "board:nami-kernelnext" || "board:rammus" || "board:sarien-kernelnext")`,
	"endorsement":             `!"betty" && !"tast_vm"`, // VMs don't have valid endorsement certificate.
	"factory_flow":            "!no_factory_flow",
	"firewall":                "!moblab", // Moblab has relaxed iptables rules
	"flashrom":                `!"betty" && !"tast_vm"`,
	"fwupd":                   "fwupd",                                  // have sys-apps/fwupd installed.
	"gboard_decoder":          "gboard_decoder",                         // have IME mojo service installed.
	"google_virtual_keyboard": "chrome_internal && internal && !moblab", // doesn't work on Moblab: https://crbug.com/949912
	"gpu_sandboxing":          `!"betty" && !"tast_vm"`,                 // no GPU sandboxing on VMs: https://crbug.com/914688
	"graphics_debugfs":        `!"kernel-3_18"`,
	"gsc":                     `"cr50_onboard" || "ti50_onboard"`,
	"hammerd":                 "hammerd",
	"houdini":                 "houdini",
	"houdini64":               "houdini64",
	"hostap_hwsim":            "wifi_hostap_test",
	"hps":                     "hps",
	"igt":                     `("video_cards_amdgpu" || "video_cards_intel" || "video_cards_mediatek" || "video_cards_msm") && !("kernel-3_18" || "kernel-4_4" || "kernel-4_14" || "kernel-4_19")`,
	"iioservice":              "iioservice",
	"inference_accuracy_eval": "inference_accuracy_eval",
	// IKEv2 is only supported on 4.19+ kernels.
	"ikev2":          `!("kernel-3_18" || "kernel-4_4" || "kernel-4_14")`,
	"iwlwifi_rescan": "iwlwifi_rescan",
	// Lacros variants.
	// veyron does not support rootfs lacros entirely. b/204888294
	// TODO(b/183969803): Remove lacros_stable and lacros_unstable eventually.
	// TODO(crbug.com/1274891): Update arm64 condition from lacros variants when becomes testable.
	"lacros":            `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger") && !arm64`,
	"lacros_stable":     `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger") && !arm64 && !"tast_vm" && !"betty"`,
	"lacros_unstable":   `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger") && !arm64 && ("tast_vm" || "betty")`,
	"lock_core_pattern": `"kernel-3_18"`,
	"manatee":           "manatee",
	"mbo":               "mbo",
	// QEMU has implemented memfd_create, but we haven't updated
	// to a release with the change (https://bugs.launchpad.net/qemu/+bug/1734792).
	// Remove "|| betty || tast_vm" from list when we upgrade.
	"memfd_create": `!("betty" || "tast_vm")`,
	"memd":         "memd",
	// Only official builds are considered to have metrics consent.
	// See: ChromeCrashReporterClient::GetCollectStatsConsent()
	// Also metrics consent needs TPM (crbug.com/1035197).
	"metrics_consent":           "chrome_internal && !mocktpm && !tast_vm",
	"microcode":                 `!"betty" && !"tast_vm"`,
	"ml_benchmark_drivers":      "ml_benchmark_drivers",
	"ml_service":                "ml_service",
	"modemfwd":                  "modemfwd",
	"mosys":                     `!"betty" && !"tast_vm"`,
	"nacl":                      "nacl",
	"ndk_translation":           "ndk_translation",
	"ndk_translation64":         "ndk_translation64",
	"nnapi":                     "nnapi",
	"nnapi_vendor_driver":       "nnapi && !betty && !tast_vm",
	"no_android":                "!arc",
	"no_arc_userdebug":          "!(cheets_userdebug || cheets_userdebug_64)",
	"no_arc_x86":                "!(amd64 && cheets_user)",
	"no_arm":                    "!arm",
	"no_asan":                   "!asan",
	"no_ath10k_4_4":             `!("board:scarlet" && "kernel-4_4")`, // board scarlet with kernel 4.4 has a version of ath10k without certain features.
	"no_borealis_host":          "!borealis_host",
	"no_chrome_dcheck":          "!chrome_dcheck",
	"no_elm_hana_3_18":          `!((elm || hana) && "kernel-3_18")`, // board elm/hana with kernel-3.18 has issue performing WiFi scan: https://crbug.com/1015719
	"no_eth_loss_on_reboot":     `!("board:jacuzzi")`,                // some devices (jacuzzi) may not enumerate eth on reboot b/178529170
	"no_iioservice":             "!iioservice",
	"no_kernel_upstream":        `!"kernel-upstream"`,
	"no_manatee":                "!manatee",
	"no_msan":                   "!msan",
	"no_ondevice_handwriting":   "!ml_service || !ondevice_handwriting",
	"no_qemu":                   `!"betty" && !"tast_vm"`,
	"no_symlink_mount":          "!lxc", // boards using LXC set CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT=n
	"no_tablet_form_factor":     "!tablet_form_factor",
	"no_tpm_dynamic":            "!tpm_dynamic",
	"no_ubsan":                  "!ubsan",
	"no_vulkan":                 "!vulkan",
	"nvme":                      "nvme",
	"oci":                       "containers && !moblab", // run_oci doesn't work on Moblab: https://crbug.com/951691
	"ocr":                       "ocr",
	"ondevice_document_scanner": "ml_service && ondevice_document_scanner",
	"ondevice_handwriting":      "ml_service && ondevice_handwriting",
	"ondevice_speech":           "ml_service && ondevice_speech",
	"pinweaver":                 `"cr50_onboard" || "pinweaver_csme" || ("tpm2_simulator" && "tpm2")`,
	"play_store":                `arc && !("board:novato" || "board:novato-arc64" || "board:novato-arcnext")`,
	"plugin_vm":                 "pita", // boards that can run Plugin VM.
	"proprietary_codecs":        "chrome_internal || chrome_media",
	"protected_content":         "cdm_factory_daemon",
	// These boards don't support pstore: https://crbug.com/971899
	"pstore": `!"betty" && !"tast_vm"`,
	"qemu":   `"betty" || "tast_vm"`,
	"racc":   "racc",
	// weird missing-runner-after-reboot bug: https://crbug.com/909955
	// TODO(yich): This is a workaround to enable reboot flag on all boards.
	// We should disable this flag if the weird missing-runner-after-reboot bug still happening.
	// Or cleanup all reboot dependency in tast-tests.
	// Notice: The flag would be false when a board didn't have any attributes.
	"reboot":                    `"*"`,
	"rrm_support":               `!("kernel-3_18" || "kernel-4_4")`,
	"screenshot":                "!rk3399", // screenshot command broken on RK3399: https://crbug.com/880597
	"selinux":                   "selinux",
	"selinux_current":           "selinux && !selinux_experimental",
	"selinux_experimental":      "selinux && selinux_experimental",
	"shill-wifi":                "!moblab", // fizz-moblab disables the WiFi technology for Shill
	"sirenia":                   "sirenia && !manatee",
	"smartdim":                  "smartdim",
	"smartctl":                  "nvme || sata",
	"storage_wearout_detect":    `"storage_wearout_detect" && !"betty" && !"tast_vm"`, // Skip wearout checks for VMs and eMMC < 5.0
	"tablet_form_factor":        "tablet_form_factor",
	"thread_safe_libva_backend": "video_cards_amdgpu || video_cards_iHD",
	"tpm":                       "!mocktpm",
	"tpm1":                      "!mocktpm && !tpm2", // Indicate tpm1.2 is available
	"tpm2":                      "!mocktpm && tpm2",  // Indicate tpm2 is available
	"tpm2_simulator":            "tpm2_simulator",
	"tpm_dynamic":               "tpm_dynamic",
	"transparent_hugepage":      "transparent_hugepage",
	"unibuild":                  "unibuild",
	"untrusted_vm":              `"kernel-4_19" || "kernel-5_4" || "kernel-5_10"`,
	"usbguard":                  "usbguard",
	"use_fscrypt_v1":            `!"direncription_allow_v2" && !"lvm_stateful_partition"`,
	"use_fscrypt_v2":            `"direncription_allow_v2" && !"lvm_stateful_partition"`,
	"v4l2_codec":                "v4l2_codec",
	"vaapi":                     "vaapi",
	// TODO(b/215374984) Remove `video_cards_ihd`.
	"video_cards_ihd": "video_cards_iHD",
	// As the direct video decoder is transitioned in there is the need
	// to run the legacy decoder to make sure it isn't broken and can be
	// rolled back to if the direct decoder is having problems.  On some
	// newer platforms there will not be a legacy decoder to run.
	"video_decoder_direct":           "!disable_cros_video_decoder",
	"video_decoder_legacy":           "disable_cros_video_decoder",
	"video_decoder_legacy_supported": `!("board:strongbad" || "board:trogdor" || "board:trogdor-kernelnext")`,
	// drm_atomic is a necessary but not sufficient condition to support
	// video_overlays; in practice, they tend to be enabled at the same time.
	// Generally you should use the more restrictive hwdep.SupportsNV12Overlays().
	"video_overlays": "drm_atomic",
	// virtual_susupend_time_injection swdep is used to limit the arc.Suspend.* tests to
	// run only on the boards that supports KVM virtual suspend time injection.
	// TODO(b/202091291): Remove virtual_susupend_time_injection swdep once it is supported
	// on all boards.
	"virtual_susupend_time_injection": `amd64 && ("kernel-4_19" || "kernel-5_4" || "kernel-5_10")`,
	"virtual_usb_printer":             `!"kernel-4_4"`,
	// Some VM builds actually can run nested VM with right host configuration.
	// But we haven't enable this feature on builders. For now, just disable
	// vm_host feature for VM builds.
	// Also, the peculiar configuration of manatee boards does not yet qualify
	// them as properly VM-enabled boards so we disable this b/219865862
	"vm_host": "kvm_host && !tast_vm && !manatee",
	// VPD is not available in VMs.
	"vpd":      `!"betty" && !"tast_vm"`,
	"vulkan":   "vulkan",
	"watchdog": `watchdog`,
	// nyan_kitty is skipped as its WiFi device is unresolvably flaky (crrev.com/c/944502),
	// exhibiting very similar symptoms to crbug.com/693724, b/65858242, b/36264732.
	"wifi":        `!"betty" && !"tast_vm" && !"nyan_kitty"`,
	"wilco":       "wilco",
	"wired_8021x": "wired_8021x",
	// WireGuard is only supported on 5.10+ kernels.
	"wireguard": `!("kernel-3_18" || "kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4")`,
	"wpa3_sae":  "wpa3_sae",
}
