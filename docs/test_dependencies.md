# Tast Test Dependencies (go/tast-deps)

A test may specify software or hardware features that must be supported on the DUT
in order for the test to run successfully. If one or more features aren't
supported by the DUT, the test will usually be skipped. See the `tast` command's
`-checktestdeps` flag to control this behavior.

Tests specify dependencies through the `SoftwareDeps` and `HardwareDeps` fields in [testing.Test].

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Test

## Software dependencies

### Existing features

The following software features are defined:

* `amd64` - The [amd64] processor architecture.
* `amd_cpu` - Boards that contain an AMD application processor.
* `android_vm` - The ability to [run Android apps] in VM instead of container.
    Any version of Android R+ can be used. Prefer this over `android_vm_r` if possible.
* `android_vm_r` - `android_vm` feature that runs in R VM.
* `android_p` - The ability to [run Android apps] that require Android P.
* `arc` - The ability to [run Android apps] in any way, in VM or container,
    with any Android version. This is intended to be used to run non-ARC tests
    only when ARC is supported on the board.
* `arc32` - Runs 32-bit Android primary ABI.
* `arc64` - Runs 64-bit Android primary ABI, may or may not have 32-bit support.
* `arc_android_data_cros_access` - The ability to access Android's /data directory from ChromeOS.
    For ARCVM virtio-blk /data devices, this only ensures that we can mount the virtio-blk disk
    image of Android's /data directory on the host, and does not grant an automatic access to
    Android's /data directory from the host side.
* `arc_camera3` - Using [Camera HAL3] interface in Chrome and Android.
* `arc_launched_32bit` - This platform originally launched with 32-bit Android.
* `arc_launched_64bit` - This platform originally launched with 64-bit Android.
* `arm` - The [arm] 32 and 64 bit processor architecture.
* `aslr` - Address space layout randomization, which mitigates buffer-overflow
    attacks, is functional (this is not true for builds with [AddressSanitizer]
    instrumentation built in).
* `auto_update_stable` - No suffix like -kernelnext and -arc-r in the board name.
    Auto update to a stable version is possible.
* `autotest-capability:foo` - An [Autotest capability] named `foo`. See below.
* `biometrics_daemon` - The ability to process fingerprint authentication.
    This implies the presence of the `biod` package.
* `bluetooth_floss` - Excludes boards that do not support the floss bluetooth adapter.
* `boot_perf_info` - Reven (ChromeOS Flex) doesn't support boot performance metrics.
* `borealis_host` - Boards that can host the Borealis system.
* `borealis_nvidia` - Boards that use nvidia supported Borealis system.
* `bpf` - Whether the platform (kernel) has the bpf syscall enabled.
* `breakpad` - Whether the platform supports the breakpad crash handler
    for Chrome.
* `camera_720p` - The ability to capture video with frame size 1280x720.
* `camera_app` - The ability to run the builtin camera app.
* `camera_feature_auto_framing` - Whether Auto Framing is enabled on this platform.
* `camera_feature_effects` - Whether camera effects are enabled on this platform.
* `camera_feature_hdrnet` - Whether HDRnet is enabled on this platform.
* `camera_feature_portrait_mode` - Whether Portrait Mode is enabled on this platform.
* `cert_provision` - The ability to use an additional cert_provision library
    that supports an interface for provisioning machine-wide certificates and
    using them for signing data on top of cryptohome dbus interface.
* `chrome` - Support for performing user login via Chrome (i.e. using
    `session_manager` and `cryptohome`). This also implies that the
    [chromeos-chrome] Portage package is installed (which also installs Chrome
    binary tests), and that the `ui` Upstart job is present.
* `chromeless` - Explicit *lack* of support for login via Chrome.
* `chromeos_firmware` - Boards that use ChromeOS firmware.
* `chromeos_kernelci` - Whether the image built for https://chromeos.kernelci.org/ or not.
* `chrome_internal` - Functionality that is only available in internal builds
    of Chrome (e.g. official branding). Any test that specifies this dependency
    should also explicitly specify a `chrome` dependency.
* `coresched` - Whether device supports core scheduling feature for secure HT.
* `cpuidle_teo` - Availability of the TEO cpuidle governor.
* `cpu_heterogeneous` - Whether device has heterogeneous CPU topology.
* `cpu_vuln_sysfs` - Whether the platform has /sys/devices/system/cpu/vulnerabilities sysfs files
* `cras` - Whether the platform supports the ChromeOS Audio Server.
* `crashpad` - Whether the platform supports the crashpad crash handler for
    Chrome.
* `cros_internal` - Functionality that is only available in internal builds of
    ChromeOS (i.e. ones built using `chromeos-overlay`).
* `crossystem` - ChromeOS firmware/system interface utility.
* `crostini_stable` - Boards that can run Crostini tests reliably.
* `crostini_unstable` - Boards that cannot run Crostini tests reliably.
* `crosvm_gpu` - Boards that use hardware GPU acceleration in the guest VM environment.
* `crosvm_no_gpu` - Boards that use software GPU emulation in the guest VM environment.
* `crosvm_swap` - Boards that have crosvm's vmm-swap feature enabled.
* `cups` - CUPS daemon.
* `device_crash` - Boards that can recover gracefully after a hard crash (e.g.
    kernel crash)
* `diagnostics` - Boards that contain generic cross-platform
    [Diagnostic utilities].
* `dlc` - Support of [Downloadable Content] (DLC).
* `dmverity_stable` - Kernels with which dm-verity runs stably. See [b/172227689](https://b.corp.google.com/issues/172227689).
* `dmverity_unstable` - Kernels having known issue of dm-verity causing random crashes. See [b/172227689](https://b.corp.google.com/issues/172227689).
* `dptf` - Support of [Intel Dynamic Platform and Thermal Framework] (DPTF).
* `drivefs` - Google Drive support enabled.
* `drm_atomic` - The [DRM/KMS] kernel subsystem supports atomic commits.
* `drm_trace` - The [DRM/KMS] kernel subsystem supports tracing using tracefs.
* `ec_crash` - Boards that have EC firmware, implement the `crash` EC command,
    and produce a panicinfo file after a crash.
* `endorsement` - Whether the system have a valid endorsement certificate.
* `faceauth` - Whether the system has faceauth functionality enabled, go/cros-faceauth.
* `factory_flow`- Device is subject to the [go/chromeos-factory-flow](http://go/chromeos-factory-flow) (e.g. most devices).
* `fake_hps` - Whether the HPS daemon (go/cros-hps) uses a fake software
    device for testing purposes.
* `firewall` - Standard ChromeOS network firewall rules.
* `flashrom` - Userspace utility to update firmware.
* `flex_id` - Enable using [flex_id](http://go/chromeos-flex-id) for enrollment.
* `ghostscript` - Ghostscript and dependent packages are available for printing.
* `google_virtual_keyboard` - The proprietary Google onscreen virtual keyboard
    (as opposed to the builtin open-source virtual keyboard).
* `gpu_sandboxing` - Chrome's GPU process is [sandboxed].
* `gsc` - Whether the platform has an onboard Google security chip.
* `hibernate`- Whether the platform supports hibernate.
* `houdini` - Availability of 32-bit Houdini library for ARC.
* `houdini64` - Availability of 64-bit Houdini library for ARC.
* `hostap_hwsim` - Whether system has the hostap project's test dependencies
    (scripts, daemons) installed and configured appropriately.
* `hps` - Whether the system has the hps daemon and tools, go/cros-hps.
* `igt` - Boards that can run igt-gpu-tools tests
* `iioservice` - Whether the device has CrOS IIO Service running.
* `inference_accuracy_eval` - Whether the device has inference accuracy evaluation tools installed.
* `intel_psr` - Whether Intel PSR (platform service record) is supported or not.
* `ikev2` - The ability to run an IKEv2 VPN.
* `io_uring` - Whether the platform (kernel) has the io_uring syscalls enabled.
* `iwlwifi_rescan` - Ability to remove/rescan WiFi PCI device when the
    hardware becomes non-responsive.
* `known_fixed_ssd` - Set when the fixed SSD location globs are defined. They are not for board:reven (the hardware is not fixed) and VMs, since the path
    is either not defined (if taken directly from xxx-generic overlay) or already defined to real hardware location.
    Only for betty we do defined a "hardware" path since that board is only used for VM.
* `lacros` - Whether the system supports running [lacros].
* `lacros_stable` - Whether the system supports running [lacros] and is stable enough for CQ. [TODO: Remove this.](crbug.com/1412276)
* `lacros_unstable` - Whether the system supports running [lacros] and is not stable enough for CQ. [TODO: Remove this.](crbug.com/1412276)
* `landlock_enabled` - Whether the system supports the Landlock Linux Security Module.
* `mbo` - WiFi MBO support.
* `memfd_create` - memfd_create function implemented in the kernel.
* `memd` - [Memory stats collection daemon].
* `microcode` - Platforms that have CPU microcode.
* `ml_service` - ML Service daemon.
* `ml_benchmark_drivers` - [ML benchmarking suite](http://go/roadrollerda)
* `mosys` - Ability to run mosys command.
* `nacl` - Availability of the Native Client sandboxing technology.
* `ndk_translation` - Availability of 32-bit NDK translation library for ARC.
* `ndk_translation64` - Availability of 64-bit NDK translation library for
    ARC.
* `nnapi` - Has the nnapi (libneuralnetworks.so) installed. Run minimal VTS tests.
* `nnapi_vendor_driver` - Run the full VTS / CTS test suite. Ignores VM's.
* `no_android` - The inability to run Android apps. This is the opposite of
    the `android` feature; DUTs will have exactly one of these two features.
* `no_arm` - The absence of [arm] 32 bit userspace.
* `no_asan` - Build was not built with Address Sanitizer. Similar to `aslr`.
* `no_ath10k_4_4` - Skip boards using the ath10k/ar10k driver on kernel 4.4, as they are missing certain features (b/138406224).
* `no_borealis_host` - Boards which is not designed to host borealis.
* `no_chrome_dcheck` - Chrome/Chromium was not built with dcheck enabld.
* `no_eth_loss_on_reboot` - Board does not lose ethernet on reboot. Context: b/178529170
* `no_iioservice` - Build was not built with CrOS IIO Service.
* `no_msan` - Build was not built with Memory Sanitizer.
* `no_ondevice_handwriting` - Doesn't have on-device handwriting recognition support. Either ml_service is not enabled, or if ml_service doesn't support `ondevice_handwriting`.
* `no_arc_userdebug` - Skip boards that ship ARC userdebug build.
* `no_arc_x86` - Skip on x86 architecture.
* `no_qemu` - For tests not for ChromeOS QEMU images.
* `no_symlink_mount` - Symlink mounting is disabled via the
    `CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT` kernel option.
* `no_tablet_form_factor` - The device's primary form factor is not tablet
* `no_tpm2_simulator` - Built was not built with the simulator of Trusted Platform Module supporting TPMv2.
* `no_tpm_dynamic` - Build was not built with dynamic TPM.
* `no_ubsan` - Build was not built with Undefined Behavior Sanitizer.
* `no_vulkan` - Build was not built with [Vulkan] enabled.
* `no_arcvm_virtio_blk_data` - Build was not built with ARCVM virtio-blk /data enabled.
* `no_gsc` - Build was not built with onboard Google security chip support.
* `nvme` - Ability to run NVMe software utilities.
* `oci` - The ability to use the `run_oci` program to execute code within
    [OCI] containers.
* `ocr` - [Optical Character Recognition Service] daemon.
* `ondevice_document_scanner` - On-device document scanner support in `ml_service` which installed document scanner library in rootfs.
    This implies `ml_service`.
* `ondevice_document_scanner_rootfs_or_dlc` - On-device document scanner support in `ml_service`.
    This implies `ml_service`.
* `ondevice_grammar` - On-device grammar check support in `ml_service`.
* `ondevice_handwriting` - On-device handwriting recognition support in `ml_service`.
* `ondevice_speech` - On-device speech recognition support in `ml_service`.
* `ondevice_text_suggestions` - On-device txt suggestions support in `ml_service`.
* `pinweaver` - Pinweaver support, either by GSC or Intel CSME.
    This implies `ml_service`.
* `play_store` - Boards where Google Play Store is supported.
* `plugin_vm` - The ability to run Plugin VMs.
* `proprietary_codecs` - Indicates if Chrome supports proprietary video
    codecs (e.g. H.264). This is supported by Chrome official builds and Chromium
    builds with the |propietary_codecs| build flag set.
* `protected_content` - Platform has HW backed OEMCrypto implementation for Widevine
    L1 HW DRM.
* `qemu` - For tests exclusive to ChromeOS QEMU images.
* `racc` - Whether [Runtime AVL Compliance Check] is available.
* `reboot` - The ability to reboot reliably during a remote test.
* `rrm_support` - Driver support for 802.11k RRM.
* `screenshot` - The [screenshot command] can save screenshots.
* `selinux` - An SELinux-enabled board. All Android boards are
    SELinux-enabled.
* `selinux_current` - All SELinux-enabled boards except experimental boards.
    This implies `selinux`.
* `selinux_experimental` - An experimental SELinux board. An experimental
    board has `SELINUX=permissive` in `/etc/selinux/config`, thus no policy
    will be enforced. This implies `selinux`.
* `shill-wifi` - WiFi technology is enabled for Shill.
* `siernia` - Sirenia is present on a non-ManaTEE image.
* `smartdim` - Use smart dim to defer the imminent screen dimming.
* `smartctl` - Ability to run smartctl software utility.
* `stack_sampled_metrics` - Ability to run the stack profiler and gather
stack-sampled metrics.
* `storage_wearout_detect` - The ability to measure storage device health.
* `tablet_form_factor` - The device's primary form factor is tablet
* `tast_vm` - The test is running in a VM [managed by chromite](https://chromium.googlesource.com/chromiumos/chromite/+/HEAD/lib/cros_test.py#396).
* `tflite_opencl` - Skips boards which have issues with the OpenCL TFLite
    delegate (b/233851820).
* `thread_safe_libva_backend` - Boards where the LIBVA backend is threadsafe.
* `tpm` - A [Trusted Platform Module] chip.
* `tpm_clear_allowed` - The device's TPM can be cleared.
* `tpm1` - Indicate a Trusted Platform Module supporting TPMv1.2 is available. Note that TPMv2 is not backward compatible.
* `tpm2` - Indicate a Trusted Platform Module supporting TPMv2 is available.
* `tpm2_simulator` - Indicate the simulator of Trusted Platform Module supporting TPMv2 is available.
* `tpm_dynamic` - Indicate the dynamic TPM is available.
* `transparent_hugepage` - [Transparent Hugepage] support in the Linux kernel.
* `unibuild` - The ChromeOS build is a unified build.
* `untrusted_vm` - The ability to run an untrusted VM.
* `usbguard` - The ability to allow or block USB devices based on policy.
* `usb_hid_wake` - The platform will register HID wake events to a usb device.
* `use_fscrypt_v1` - The board is set to use v1 fscrypt policy for user vault.
* `use_fscrypt_v2` - The board is set to use v2 fscrypt policy for user vault.
* `uvc_compliant` - The board has a UVC driver that is V4L2 compliant.
* `v4l2_codec` - Whether or not v4l2 video acceleration API is supported by this DUT.
* `vaapi` - Whether or not VA-API is supported by this DUT.
* `video_cards_ihd` - Boards that use the Intel Media Driver (also known as iHD) for VA-API.
* `video_decoder_direct` - The platform uses the VideoDecoder (VD) by default.
* `video_decoder_legacy` - The platform used the VideoDecodeAccelerator (VDA) by default.
* `video_decoder_legacy_supported` - Is the VDA is supported on this platform.
* `video_overlays` - The kernel [DRM/KMS] version atomic commits and the underlying hardware display controller support the NV12 DRM Plane format needed to promote videos to [hardware overlays].
* `virtual_multidisplay` - The platform supports virtual multi display testing (at the time of writing, only 6.1+ betty boards).  This should be depended on for all multidisplay presubmit tests to run on betty.
* `virtual_susupend_time_injection` - The platform supports KVM virtual suspend time injection.
* `virtual_usb_printer` - Whether or not the device can run tests that
    use [virtual USB printing][virtual-usb-printer-readme]. Note that
    while the necessary kernel modules are available on kernel v4.4,
    this feature excludes that version for known flakiness. See
    [this bug](https://b.corp.google.com/issues/172224081) for context.
* `vm_host` - The ability to [run virtual machines].
* `vpd` - The DUT has a VPD chip.
* `vulkan` - Whether [Vulkan] libraries are installed on the system image.
* `vulkan_composite` - Boards that support composition with [Vulkan].
* `watchdog` - watchdog daemon
* `wifi` - If this DUT has WiFi device.
* `wpa3_sae` - The ability to use WPA3-SAE authentication for WiFi.
* `wilco` - If this DUT is a [wilco] device. These features include
    the DTC (Diagnostic and Telemetry Controller) VM, a special EC interface,
    and a dock firmware updater.
* `wireguard` - The ability to run a WireGuard VPN.
* `no_kernel_upstream` - Skip boards with continuously-rebased kernel.

[amd64]: https://en.wikipedia.org/wiki/X86-64
[arm]: https://en.wikipedia.org/wiki/ARM_architecture
[run Android apps]: https://developer.android.com/topic/arc/
[Camera HAL3]: https://source.android.com/devices/camera/camera3
[Camera HAL1]: https://source.android.com/devices/camera#architecture-legacy
[Linux Video Capture]: https://chromium.googlesource.com/chromium/src/+/HEAD/media/capture/video/linux/
[AddressSanitizer]: https://github.com/google/sanitizers/wiki/AddressSanitizer
[Autotest capability]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/main/chromeos-base/autotest-capability-default/
[chromeos-chrome]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/main/chromeos-base/chromeos-chrome/chromeos-chrome-9999.ebuild
[media::VideoDecoder]: https://cs.chromium.org/chromium/src/media/base/video_decoder.h
[Diagnostic utilities]: https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/diagnostics/README.md
[Downloadable Content]: https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/dlcservice
[DRM/KMS]: https://www.kernel.org/doc/Documentation/gpu/drm-kms.rst
[hardware overlays]: https://en.wikipedia.org/wiki/Hardware_overlay
[Intel Dynamic Platform and Thermal Framework]: https://01.org/intel%C2%AE-dynamic-platform-and-thermal-framework-dptf-chromium-os
[lacros]: https://chromium.googlesource.com/chromium/src.git/+/HEAD/docs/lacros.md
[sandboxed]: https://chromium.googlesource.com/chromium/src/+/HEAD/docs/linux_sandboxing.md
[Memory stats collection daemon]: https://chromium.googlesource.com/chromiumos/platform2/+/main/metrics/memd/
[OCI]: https://www.opencontainers.org/
[Optical Character Recognition Service]: https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/ocr/README.md
[Runtime AVL Compliance Check]: https://chromium.googlesource.com/chromiumos/platform2/+/refs/heads/main/runtime_probe/README.md
[screenshot command]: https://chromium.googlesource.com/chromiumos/platform2/+/main/screen-capture-utils/
[Trusted Platform Module]: https://en.wikipedia.org/wiki/Trusted_Platform_Module
[Transparent Hugepage]: https://www.kernel.org/doc/Documentation/vm/transhuge.txt
[run virtual machines]: https://chromium.googlesource.com/chromiumos/docs/+/main/containers_and_vms.md
[Vulkan]: https://www.khronos.org/vulkan/
[virtual-usb-printer-readme]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/virtual-usb-printer/README.md
[wilco]: https://sites.google.com/corp/google.com/wilco/home

### New features

Features should be descriptive and precise. Consider a hypothetical test that
exercises authentication using a biometrics daemon that isn't present in system
images built to run on virtual machines. Instead of adding a `real_hardware` or
`non_vm` feature that is overly broad and will likely be interpreted as carrying
additional meaning beyond the original intent, add a `biometrics_daemon` feature
that precisely communicates the test's actual requirement.

Features are composed from USE flags and board names, which are statically
defined when the system image is built. [software_defs.go] lists boolean
expressions that are used to generate features; for example, an imaginary
feature named `hd_audio` with the expression

```go
cras && (audio_chipset_a || audio_chipset_b) && !broken_headphone_jack
```

will be reported as available on systems where the `cras` USE flag is set,
either `audio_chipset_a` or `audio_chipset_b` is set, and
`broken_headphone_jack` is explicitly *not* set.

A feature can depend on board names, too. Another imaginary feature
named `vm_graphics` with the expression

```go
"board:betty-pi-arc"
```

will be reported as available on `betty-pi-arc` board only.

Before a new `USE` flag can be used in an expression, it must be added to `IUSE`
in the [tast-use-flags] package. Local changes to the `tast-use-flags` ebuild
have to be pushed to the DUT manually to take effect:

```
cros_workon-$BOARD start chromeos-base/tast-use-flags
emerge-$BOARD chromeos-base/tast-use-flags
cros deploy --root=/usr/local $HOST chromeos-base/tast-use-flags
```

When submitting changes to add new `USE` flags to the [tast-use-flags] package,
please use [Cq-Depend] in your commit messages to ensure that changes land in
the correct order.

If you're having trouble finding a way to specify your test's dependencies,
please ask for help on the [tast-users mailing list].

[software_defs.go]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/go.chromium.org/tast/core/internal/crosbundle/software_defs.go
[tast-use-flags]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/main/chromeos-base/tast-use-flags/
[Cq-Depend]: https://chromium.googlesource.com/chromiumos/docs/+/main/contributing.md#cq-depend
[tast-users mailing list]: https://groups.google.com/a/chromium.org/g/tast-users

#### Example changes

See the following changes for an example of adding a new `containers` software
feature based on the `containers` USE flag and making a test depend on it:

*   `chromiumos-overlay` repository: <https://crrev.com/c/1382877>
*   `tast` repository: <https://crrev.com/c/1382621>
*   `tast-tests` repository: <https://crrev.com/c/1382878>

(Note that the `containers` feature has since been renamed to `oci`.)

### autotest-capability

There are also `autotest-capability:`-prefixed features, which are added by the
[autocaps package] as specified by YAML files in
`/usr/local/etc/autotest-capability`. This exists in order to support porting
existing Autotest-based video tests to Tast. Do not depend on capabilities from
outside of video tests.

[autocaps package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/autocaps/


## Hardware dependencies

Tast provides a way to run/skip tests based on the device characteristics.

Note that "device characteristics" here only consists of information that can
be determined solely based on the DUT, without depending on the other
surrounding environment, such as some config files on DUT.

The examples of the device characteristics are as follows:

* Whether the device has a touch screen.
* Whether the device has fingerprint.
* Whether the device has an internal display.

For example, in order to run tests on DUT where touchscreen is available,
the dependency can be declared in the `HardwareDeps` field of `testing.Test`.

```go
func init() {
  testing.AddTest(&testing.Test{
    ...
    HardwareDeps: hwdep.D(hwdep.Touchscreen()),
    ...
  })
}
```

You can provide multiple `Condition`s to `hwdep.D`. In the case,
the test will run only on DUTs where all the conditions are satisfied.

You can find the full list of supported conditions in the [hwdep package].

Note that there are special kinds of hardware dependencies, named `Model` and
`SkipOnModel`.
With these dependencies, tests will be controlled based on the device type names,
rather than the device characteristics.
In general, it is recommended *not* to use these conditions. If you feel you need
these conditions, it is recommended to reconsider whether there is an alternative
(and more appropriate) condition.
Examples of their expected use cases are:

* There are known issues in a driver of a specific device, which cannot be
  fixed immediately. The test is stable on other models, and we would like
  to promoted it to critical.
* There is a test running as informational. Flakiness failures are found
  only on a few models, but the test is stable on other models.
  With depending models, we can promote the test to critical on
  most of models, except ones where the test results flakiness.
  In this case, it is expected that a dedicated engineer is assigned to
  investigate the cause and its fix.

[hwdep package]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/go.chromium.org/tast/core/testing/hwdep/

### Adding new hardware conditions

In order to guarantee forward compatibility in ChromeOS infra,
each `Condition` should be based on the
`chromiumos.config.api.HardwareFeatures` protobuf schema.

For example, the `hwdep.Touchscreen()` can check
whether `Screen.TouchSupport` is set to `HardwareFeatures_PRESENT`.

Note that currently a `chromiumos.config.api.HardwareFeatures` instance is
generated internally by Tast at runtime, so only limited fields are filled.
In the future, ChromeOS Infra test scheduler will be responsible for checking
hardware dependencies before running Tast tests.

Here is an example end-to-end workflow:
Let’s assume that a developer wants to add a new Tast test which requires a new
hardware feature to be used in the test hardware constraints (e.g. “wifi chip
vendor name is X”).

1. The developer makes a change for that hardware feature.
    * The developer files a CL for a change to the schema
     [(config/api/topology.proto)][1] to add a new field for the feature.
    * A domain expert approves the change in the .proto file
     [(config/api/topology.proto)][1].  [(Example CL)][2]
1. To test locally, run these commands to regenerate the go proto bindings.
    ```
    ~/chromiumos/src/config/generate.sh
    cros workon --host start cros-config-api
    sudo emerge cros-config-api
    ```
1. The developer waits until the CL in #1 is landed, then implements
   some functions in the Tast framework supporting the new feature(s) using the
   new message type.
    * The Tast team reviews and approves such a change to Tast.
    [Here is an example CL][3] which puts some data into the protobuf in Tast.
1. The developer writes test(s) with hwdeps in its test metadata using the
   above function in Tast.

[1]: https://source.chromium.org/chromium/infra/infra/+/HEAD:go/src/go.chromium.org/chromiumos/config/proto/chromiumos/config/api/topology.proto
[2]: https://chromium-review.googlesource.com/c/chromiumos/config/+/2249691/4/proto/chromiumos/config/api/topology.proto
[3]: https://chromium-review.googlesource.com/c/chromiumos/platform/tast/+/2335615
