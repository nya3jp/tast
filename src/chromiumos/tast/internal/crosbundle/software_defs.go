// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

// softwareFeatureDefs defines software features for ChromeOS test bundles.
var softwareFeatureDefs = map[string]string{
	// This list is documented at docs/test_dependencies.md.
	// All USE flags referenced here must be listed in IUSE in the tast-use-flags ebuild.
	// The one exception is tast_vm, which is inserted by VM builders via -extrauseflags.
	"amd64":   "amd64",
	"amd_cpu": "amd_cpu",
	// ARC USE flags are defined here:
	// http://cs/chromeos_public/src/third_party/chromiumos-overlay/eclass/arc-build-constants.eclass
	// TODO(b/210917721): Re-enable ARCVM on manatee.
	"android_vm":          `arc && arcvm && !"android-vm-pi" && !manatee`,
	"android_vm_r":        `arc && arcvm && "android-vm-rvc" && !manatee`,
	"android_vm_t":        `arc && arcvm && "android-vm-tm"`,
	"android_container":   `arc && ("android-container-rvc" || "android-container-pi")`,
	"android_p":           `arc && "android-container-pi"`,
	"android_container_r": `arc && "android-container-rvc"`,
	"android_r":           `arc && ("android-container-rvc" || "android-vm-rvc")`,
	"arc":                 `arc`,
	"arc32":               `"cheets_user" || "cheets_userdebug"`,
	"arc64":               `"cheets_user_64" || "cheets_userdebug_64"`,
	// To access Android's /data directory from ChromeOS on ARCVM virtio-blk /data devices by mounting the virtio-blk disk image of Android's /data directory,
	// the host kernel version needs to be 5.2 or above.
	"arc_android_data_cros_access": `!arcvm_virtio_blk_data || "kernel-5_4" || "kernel-5_10" || "kernel-5_15"`,
	"arc_camera3":                  `"arc-camera3"`,
	"arc_launched_32bit":           `"arc-launched-32bit-abi"`,
	"arc_launched_64bit":           `"!arc-launched-32bit-abi"`,
	"arm":                          `"arm" || "arm64"`,
	"aslr":                         "!asan", // ASan instrumentation breaks ASLR
	"auto_update_stable":           `!("board:*-*")`,
	"biometrics_daemon":            "biod",
	"boot_perf_info":               `!("board:reven*")`, // Reven (ChromeOS Flex) doesn't support boot performance metrics.
	// TODO(b/225065082): Re-enable borealis on manatee
	"borealis_host": "borealis_host && !manatee",
	// The bpf syscall is enabled on CrOS since kernel v5.10.
	"bpf":      `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4")`,
	"breakpad": "force_breakpad",
	// daisy variants' cameras don't support 1280x720.
	"camera_720p": "!snow && !skate && !spring",
	// Some boards might not support the camera/video/audio components required by the camera app.
	// TODO(b/185087278): Remove soraka-libcamera.
	"camera_app":                   `!("board:volteer-kernelnext" || "board:soraka-libcamera")`,
	"camera_feature_auto_framing":  "camera_feature_auto_framing",
	"camera_feature_effects":       "camera_feature_effects",
	"camera_feature_hdrnet":        "camera_feature_hdrnet",
	"camera_feature_portrait_mode": "camera_feature_portrait_mode",
	"cellular_modem_dlcs_present":  `!("board:coral*" || "board:dedede*" || "board:drallion*" || "board:hatch*" || "board:nautilus*" || "board:octopus*" || "board:sarien*" || "board:zork*")`,
	"cellular_variant_present":     `!("board:drallion*" || "board:nautilus*" || "board:sarien*")`,
	"cert_provision":               "cert_provision",
	"chrome":                       "!chromeless_tty && !rialto",
	"chrome_internal":              "chrome_internal",
	"chromeless":                   "chromeless_tty || rialto",
	"chromeos_ec_firmware":         `!"wilco" && !"betty" && !"tast_vm" && !"board:reven*"`, // Wilco devices run Dell EC firmware.  VM boards (e.g. betty) don't have EC firmware.  Reven (ChromeOS Flex) devices run third-party firmware.
	"chromeos_firmware":            `!("board:reven*")`,                                     // Reven (ChromeOS Flex) devices run third-party firmware.
	"chromeos_kernelci":            "chromeos_kernelci_builder",
	// Kernels pre-4.19 do not support core scheduling.
	"coresched": `!("kernel-4_4" || "kernel-4_14")`,
	// TEO governor was new in v5.1, but we backported it to v4.19.
	"cpuidle_teo": `!("kernel-4_4" || "kernel-4_14")`,
	// TODO(b/174889440) Remove hana, elm, kevin, bob, scarlet.
	"cpu_vuln_sysfs": `!("board:bob" || "board:hana" || "board:elm" || "board:hana-kernelnext" || "board:elm-kernelnext" || "board:kevin" || "board:scarlet")`,
	"cras":           "cras",
	"crashpad":       "!force_breakpad",
	"cros_internal":  "internal",
	// Boards to run Crostini Apps tests. See b/256521958.
	"crostini_app":  `"board:atlas" || "board:brya" || "board:coral" || "board:dedede" || "board:eve" || "board:grunt" || "board:hatch" || "board:jacuzzi" || "board:nami" || "board:octopus" || "board:scarlet" || "board:volteer" || "board:zork"`,
	"crosvm_gpu":    `"crosvm-gpu" && "virtio_gpu"`,
	"crosvm_no_gpu": `!"crosvm-gpu" || !"virtio_gpu"`,
	// VMs don't support few crossystem sub-commands: https://crbug.com/974615
	"crossystem":        `!"betty" && !"tast_vm"`,
	"cups":              "cups",
	"diagnostics":       `"diagnostics" && !"betty" && !"tast_vm"`, // VMs do not have hardware to diagnose. https://crbug.com/1126619
	"dlc":               "dlc && dlc_test",
	"dptf":              "dptf",
	"device_crash":      `!("board:samus")`, // Samus devices do not reliably come back after kernel crashes. crbug.com/1045821
	"dmverity_stable":   `"kernel-4_4" || "kernel-4_14"`,
	"dmverity_unstable": `!("kernel-4_4" || "kernel-4_14")`,
	"drivefs":           "drivefs",
	"drm_atomic":        "drm_atomic",
	"drm_trace":         `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19")`,
	// asuka, banon, caroline, cave, celes, chell, cyan, edgar, kefka, reks, relm, sentry, terra, ultima, and wizpig have buggy EC firmware and cannot capture crash reports. b/172228823
	// drallion and sarien have do not support the "crash" EC command. crbug.com/1123716
	// guado, tidus, rikku, veyron_fievel, and veyron_tiger do not have EC firmware. crbug.com/1123716. TODO(crbug.com/1124554) Use an EC hardware dep for these rather than a software dep.
	// nocturne only sporadically captures EC panics. crbug.com/1135798
	// TODO(https://crbug.com/1122066): remove guado-cfm and rikku-cfm when they're no longer necessary
	// TODO(b/201430283): Remove nami-kernelnext, rammus, and sarien-kernelnext when bug is resolved.
	"ec_crash":            `!(("board:asuka" || "board:banon" || "board:caroline" || "board:caroline-kernelnext" || "board:caroline-userdebug" || "board:cave" || "board:celes" || "board:chell" || "board:cyan" || "board:edgar" || "board:kefka" || "board:reks" || "board:relm" || "board:sentry" || "board:terra" || "board:ultima" || "board:wizpig") || ("board:drallion" || "board:sarien") || ("board:guado" || "board:guado-cfm" || "board:tidus" || "board:rikku" || "board:rikku-cfm" || "board:veyron_fievel" || "board:veyron_tiger") || "board:nocturne" || "board:nocturne-kernelnext" || "board:nami-kernelnext" || "board:rammus" || "board:sarien-kernelnext")`,
	"ec_system_safe_mode": `"board:skyrim"`,
	"endorsement":         `!"betty" && !"tast_vm"`, // VMs don't have valid endorsement certificate.
	"faceauth":            "faceauth",
	"factory_flow":        "!no_factory_flow",
	"fake_hps":            `"betty" || "tast_vm"`, // VMs run hpsd with --test (fake software device)
	// TODO(http://b/271025366): Remove feedback when the bug is resolved.
	"feedback":                  `!("board:fizz" || "board:puff" || "board:rammus")`,
	"firewall":                  "!moblab", // Moblab has relaxed iptables rules
	"flashrom":                  `!"betty" && !"tast_vm"`,
	"first_class_servo_working": `!("board:brya" || "board:volteer")`,     // TODO(b/274634861): remove the first_class_servo_working when fixed.
	"flex_id":                   "flex_id",                                // Enable using flex_id for enrollment
	"fwupd":                     "fwupd",                                  // have sys-apps/fwupd installed.
	"ghostscript":               "postscript",                             // Ghostscript and dependent packages available
	"google_virtual_keyboard":   "chrome_internal && internal && !moblab", // doesn't work on Moblab: https://crbug.com/949912
	"gpu_sandboxing":            `!"betty" && !"tast_vm"`,                 // no GPU sandboxing on VMs: https://crbug.com/914688
	"gsc":                       `"cr50_onboard" || "ti50_onboard"`,
	"hana":                      "hana",
	"hammerd":                   "hammerd",
	"houdini":                   "houdini",
	"houdini64":                 "houdini64",
	"hostap_hwsim":              "wifi_hostap_test",
	"hps":                       "hps",
	"hwdrm_stable":              `!("board:brya" || "board:geralt")`, // brya devices have FW corruption issues with HWDRM: b/243456977, geralt is under development
	"igt":                       `("video_cards_amdgpu" || "video_cards_intel" || "video_cards_mediatek" || "video_cards_msm") && !("kernel-4_4" || "kernel-4_14" || "kernel-4_19")`,
	"iioservice":                "iioservice",
	"inference_accuracy_eval":   "inference_accuracy_eval",
	// IKEv2 is only supported on 4.19+ kernels.
	"ikev2": `!("kernel-4_4" || "kernel-4_14")`,
	// The io_uring syscalls are enabled on CrOS since kernel v5.15.
	"io_uring":       `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4" || "kernel-5_10")`,
	"iwlwifi_rescan": "iwlwifi_rescan",
	// Lacros variants.
	// veyron does not support rootfs lacros entirely. b/204888294
	// TODO(crbug.com/1412276): Remove lacros_stable and lacros_unstable eventually.
	"lacros":                 `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger")`,
	"lacros_stable":          `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger") && !"tast_vm" && !"betty"`,
	"lacros_unstable":        `!chromeless_tty && !rialto && !("board:veyron_fievel" || "board:veyron_tiger") && ("tast_vm" || "betty")`,
	"landlock_enabled":       `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4")`,
	"lvm_stateful_partition": "lvm_stateful_partition",
	"manatee":                "manatee",
	"mbo":                    "mbo",
	// QEMU has implemented memfd_create, but we haven't updated
	// to a release with the change (https://bugs.launchpad.net/qemu/+bug/1734792).
	// Remove "|| betty || tast_vm" from list when we upgrade.
	"memfd_create": `!("betty" || "tast_vm")`,
	"memd":         "memd",
	// Memfd execution attempts are detected and blocked only on the following kernel versions.
	"memfd_exec_detection": `("kernel-4_19" || "kernel-5_4" || "kernel-5_10" || "kernel-5_15")`,
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
	"no_amd_cpu":                "!amd_cpu",
	"no_android":                "!arc",
	"no_arc_userdebug":          "!(cheets_userdebug || cheets_userdebug_64)",
	"no_arc_x86":                "!(amd64 && cheets_user)",
	"no_arm":                    "!arm",
	"no_asan":                   "!asan",
	"no_ath10k_4_4":             `!("board:scarlet" && "kernel-4_4")`, // board scarlet with kernel 4.4 has a version of ath10k without certain features.
	"no_borealis_host":          "!borealis_host",
	"no_chrome_dcheck":          "!chrome_dcheck",
	"no_eth_loss_on_reboot":     `!("board:jacuzzi")`,                                                                                                                                // some devices (jacuzzi) may not enumerate eth on reboot b/178529170
	"no_igt":                    `!("video_cards_amdgpu" || "video_cards_intel" || "video_cards_mediatek" || "video_cards_msm") || ("kernel-4_4" || "kernel-4_14" || "kernel-4_19")`, // opposite of "igt"
	"no_iioservice":             "!iioservice",
	"no_kernel_upstream":        `!"kernel-upstream"`,
	"no_manatee":                "!manatee",
	"no_msan":                   "!msan",
	"no_ondevice_handwriting":   "!ml_service || !ondevice_handwriting",
	"no_qemu":                   `!"betty" && !"tast_vm"`,
	"no_symlink_mount":          "!lxc", // boards using LXC set CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT=n
	"no_tablet_form_factor":     "!tablet_form_factor",
	"no_tpm2_simulator":         "!tpm2_simulator",
	"no_tpm_dynamic":            "!tpm_dynamic",
	"no_ubsan":                  "!ubsan",
	"no_vulkan":                 "!vulkan",
	"no_arcvm_virtio_blk_data":  "!(arcvm_virtio_blk_data || arcvm_data_migration)",
	"no_gsc":                    `!"cr50_onboard" && !"ti50_onboard"`,
	"nvme":                      "nvme",
	"oci":                       "containers && !moblab", // run_oci doesn't work on Moblab: https://crbug.com/951691
	"ocr":                       "ocr",
	"octopus":                   "octopus",
	"ondevice_document_scanner": "ml_service && ondevice_document_scanner",
	"ondevice_document_scanner_rootfs_or_dlc": "ml_service && (ondevice_document_scanner || ondevice_document_scanner_dlc)",
	"ondevice_grammar":                        "ml_service && ondevice_grammar",
	"ondevice_handwriting":                    "ml_service && ondevice_handwriting",
	"ondevice_speech":                         "ml_service && ondevice_speech",
	"ondevice_text_suggestions":               "ml_service && ondevice_text_suggestions",
	"pinweaver":                               `"ti50_onboard" || "cr50_onboard" || "pinweaver_csme" || ("tpm2_simulator" && "tpm2")`,
	"play_store":                              `arc && !("board:novato" || "board:novato-arc64" || "board:novato-arcnext")`,
	"plugin_vm":                               "pita", // boards that can run Plugin VM.
	"printscanmgr":                            "printscanmgr",
	"proprietary_codecs":                      "chrome_internal || chrome_media",
	"protected_content":                       "cdm_factory_daemon",
	// VM boards don't support pstore: https://crbug.com/971899
	// reven boards don't support pstore: b/234722825
	"pstore": `!("betty" || "tast_vm" || "board:reven*")`,
	// ptp_kvm is only available on ARM in kernel 5.10 or later.
	// ptp_kvm is unreliable on amd64 in kernel 4.19.
	"ptp_kvm": `("amd64" && !"kernel-4_19") || (("arm" || "arm64") && !("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4"))`,
	"qemu":    `"betty" || "tast_vm"`,
	"racc":    "racc",
	// weird missing-runner-after-reboot bug: https://crbug.com/909955
	// TODO(yich): This is a workaround to enable reboot flag on all boards.
	// We should disable this flag if the weird missing-runner-after-reboot bug still happening.
	// Or cleanup all reboot dependency in tast-tests.
	// Notice: The flag would be false when a board didn't have any attributes.
	"reboot":               `"*"`,
	"rrm_support":          `!"kernel-4_4"`,
	"selinux":              "selinux",
	"selinux_current":      "selinux && !selinux_experimental",
	"selinux_experimental": "selinux && selinux_experimental",
	"shill-wifi":           "!moblab", // fizz-moblab disables the WiFi technology for Shill
	"sirenia":              "sirenia && !manatee",
	"smartdim":             "smartdim",
	"smartctl":             "nvme || sata",
	// VMs don't support speech on-device API.
	"soda": `!"betty" && !"tast_vm"`,
	// Should match StackSamplingProfiler::IsSupportedForCurrentPlatform() in Chromium repo.
	"stack_sampled_metrics":     `"amd64" && !"betty" && !"tast_vm"`,
	"storage_wearout_detect":    `"storage_wearout_detect" && !"betty" && !"tast_vm"`, // Skip wearout checks for VMs and eMMC < 5.0
	"tablet_form_factor":        "tablet_form_factor",
	"tflite_opencl":             `!(elm || hana)`, // these boards seem to have issues with the OpenCL TFLite delegate (b/233851820)
	"thread_safe_libva_backend": "video_cards_amdgpu || video_cards_iHD",
	"tpm":                       "!mocktpm",
	"tpm_clear_allowed":         "!mocktpm && (!tpm_dynamic || tpm2_simulator)", // this filters out the reven board from TPM devices
	"tpm1":                      "!mocktpm && !tpm2",                            // Indicate tpm1.2 is available
	"tpm2":                      "!mocktpm && tpm2",                             // Indicate tpm2 is available
	"tpm2_simulator":            "tpm2_simulator",
	"tpm_dynamic":               "tpm_dynamic",
	"transparent_hugepage":      "transparent_hugepage",
	// physical_location is only supported in kernel 5.10 or later.
	// physical_location is not supported in ARM.
	// Proper physical_location is not added to older boards, but should be included in newer boards.
	"typec_physical_location": `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4") && !("arm" || "arm64") && !("board:amd*" || "board:asuka*" || "board:asurada*" || "board:atlas*" || "board:banon*" || "board:betty*" || "board:bob*" || "board:caroline*" || "board:cave*" || "board:celes*" || "board:chell*" || "board:cherry*" || "board:coral*" || "board:cyan*" || "board:dedede*" || "board:drallion*" || "board:edgar*" || "board:elm*" || "board:eve*" || "board:fizz*" || "board:grunt*" || "board:hana*" || "board:hatch*" || "board:jacuzzi*" || "board:kalista*" || "board:keeby*" || "board:kefka*" || "board:kevin*" || "board:kukui*" || "board:lars*" || "board:nami*" || "board:nautilus*" || "board:nocturne*" || "board:novato*" || "board:octopus*" || "board:puff*" || "board:pyro*" || "board:rammus*" || "board:reef*" || "board:reks*" || "board:relm*" || "board:reven*" || "board:sand*" || "board:sarien*" || "board:scarlet*" || "board:sentry*" || "board:setzer*" || "board:snappy*" || "board:soraka*" || "board:strongbad*" || "board:terra*" || "board:trogdor*" || "board:ultima*" || "board:volteer*" || "board:wizpig*" || "board:zork*")`,
	"typec_usb_link":          `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19" || "kernel-5_4") && !("arm" || "arm64")`,
	"unibuild":                "unibuild",
	"untrusted_vm":            `!("kernel-4_4" || "kernel-4_14")`,
	"usbguard":                "usbguard",
	"usb_hid_wake":            `!("board:octopus*")`,
	"use_fscrypt_v1":          `!"direncription_allow_v2" && !"lvm_stateful_partition"`,
	"use_fscrypt_v2":          `"direncription_allow_v2" && !"lvm_stateful_partition"`,
	"uvc_compliant":           `!("kernel-4_4" || "kernel-4_14")`,
	"v4l2_codec":              "v4l2_codec",
	"vaapi":                   "vaapi",
	// TODO(b/215374984) Remove `video_cards_ihd`.
	"video_cards_ihd": "video_cards_iHD",
	// As the direct video decoder is transitioned in there is the need
	// to run the legacy decoder to make sure it isn't broken and can be
	// rolled back to if the direct decoder is having problems.  On some
	// newer platforms there will not be a legacy decoder to run.
	"video_decoder_direct":           "!disable_cros_video_decoder",
	"video_decoder_legacy":           "disable_cros_video_decoder",
	"video_decoder_legacy_supported": `!("board:strongbad*" || "board:trogdor*" || "board:herobrine*" || "board:corsola*")`,
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
	"wifi":  `!"betty" && !"tast_vm" && !"nyan_kitty"`,
	"wilco": "wilco",
	// WireGuard is only supported on 5.4+ kernels.
	"wireguard": `!("kernel-4_4" || "kernel-4_14" || "kernel-4_19")`,
	"wpa3_sae":  "wpa3_sae",
	// Some boards cannot play/record stably, disabling these tests and keeping
	// some of them informationl.
	// TODO(b/240269271): remove "octopus" and "hatch" when b/240269271 is fixed.
	"audio_stable":               `!("board:octopus" || "board:hatch")`,
	"audio_unstable":             `"board:octopus" || "board:hatch"`,
	"gsc_can_wake_ec_with_reset": `!("board:grunt" || "board:nami")`,
	"ec_hibernate":               `!("board:brask" || "board:fizz" || "board:kukui" || "board:puff" || "board:scarlet" || "board:shotzo")`,
}
