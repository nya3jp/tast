# Tast Test Dependencies (go/tast-deps)

A test may specify software features that must be supported by the DUT's system
image in order for the test to run successfully. If one or more features aren't
supported by the DUT, the test will usually be skipped. See the `tast` command's
`-checktestdeps` flag to control this behavior.

Tests specify dependencies through the `SoftwareDeps` field in [testing.Test].

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test

## Existing features

The following software features are defined:

*   `alt_syscall` - Whether the platform supports the alt syscall framework.
*   `amd64` - The [amd64] processor architecture.
*   `android` - The ability to [run Android apps]. Any production version of
    Android (i.e. ones except `master-arc-dev`) can be used.
*   `android_vm` - `android` feature that runs in vm instead of container.
*   `android_both` - `android` feature that runs in both vm and container.
*   `android_all` - in addition to `android`, runs on `master-arc-dev`, too.
*   `android_all_both` - `android_all` feature that runs in both vm and container.
*   `android_p` - The ability to [run Android apps] that require Android P or
    later.
*   `android_p_both` - `android_p` feature that runs in both vm and container.
*   `arc_camera3` - The [Camera HAL3] interface in Android.
*   `aslr` - Address space layout randomization, which mitigates buffer-overflow
    attacks, is functional (this is not true for builds with [AddressSanitizer]
    instrumentation built in).
*   `audio_play` - The ability to play audio.
*   `audio_record` - The ability to record audio.
*   `autotest-capability:foo` - An [Autotest capability] named `foo`. See below.
*   `biometrics_daemon` - The ability to process fingerprint authentication.
    This implies the presence of the `biod` package.
*   `camera_720p` - The ability to capture video with frame size 1280x720.
*   `chrome` - Support for performing user login via Chrome (i.e. using
    `session_manager` and `cryptohome`). This also implies that the
    [chromeos-chrome] Portage package is installed (which also installs Chrome
    binary tests), and that the `ui` Upstart job is present.
*   `chrome_internal` - Functionality that is only available in internal builds
    of Chrome (e.g. official branding and proprietary codecs like H.264). Any
    test that specifies this dependency should also explicitly specify a
    `chrome` dependency.
*   `cros_config` - `cros_config` utility is available.
*   `cros_internal` - Functionality that is only available in internal builds of
    Chrome OS (i.e. ones built using `chromeos-overlay`).
*   `crossystem` - Chrome OS firmware/system interface utility.
*   `crosvm_gpu` - The ability to use hardware GPU acceleration in the guest VM environment.
*   `cups` - CUPS daemon.
*   `diagnostics` - Boards that contain generic cross-platform
    [Diagnostic utilities].
*   `display_backlight` - An internal display backlight.
*   `dlc` - Support of [Downloadable Content] (DLC).
*   `drm_atomic` - The ability to synchronize video buffer overlays atomically.
    This is guarantees that [video hardware overlays] are supported.
*   `firewall` - Standard Chrome OS network firewall rules.
*   `google_virtual_keyboard` - The proprietary Google onscreen virtual keyboard
    (as opposed to the builtin open-source virtual keyboard).
*   `gpu_sandboxing` - Chrome's GPU process is [sandboxed].
*   `gsc` - Whether the platform has an onboard Google security chip.
*   `memd` - [Memory stats collection daemon].
*   `ml_service` - ML Service daemon.
*   `mosys` - Ability to run mosys command.
*   `no_android` - The inability to run Android apps. This is the opposite of
    the `android` feature; DUTs will have exactly one of these two features.
*   `no_asan` - Build was not built with Address Sanitizer. Similar to `aslr`.
*   `no_symlink_mount` - Symlink mounting is disabled via the
    `CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT` kernel option.
*   `oci` - The ability to use the `run_oci` program to execute code within
    [OCI] containers.
*   `qemu` - For tests exclusive to Chrome OS QEMU images.
*   `reboot` - The ability to reboot reliably during a remote test.
*   `screenshot` - The [screenshot command] can save screenshots.
*   `selinux` - An SELinux-enabled board. All Android boards are
    SELinux-enabled.
*   `selinux_current` - All SELinux-enabled boards except experimental boards.
    This implies `selinux`.
*   `selinux_experimental` - An experimental SELinux board. An experimental
    board has `SELINUX=permissive` in `/etc/selinux/config`, thus no policy
    will be enforced. This implies `selinux`.
*   `smartdim` - Use smart dim to defer the imminent screen dimming.
*   `tablet_mode` - The ability to enter tablet mode. The device is either
    a convertible device or a tablet device.
*   `tpm` - A [Trusted Platform Module] chip.
*   `transparent_hugepage` - [Transparent Hugepage] support in the Linux kernel.
*   `usbguard` - The ability to allow or block USB devices based on policy.
*   `virtual_usb_printer` - Emulates a USB printer. This implies the presence of
    the `usbip` program.
*   `vm_host` - The ability to [run virtual machines].
*   `vp9_sanity` - The ability to stay alive playing a VP9 video with hardware
    acceleration even for a profile which the driver doesn't support.
*   `vulkan` - Whether [Vulkan] is enabled.
*   `wilco` - If this DUT is a [wilco] device. These features include
    the DTC (Diagnostic and Telemetry Controller) VM, a special EC interface,
    and a dock firmware updater.
*   `wifi` - If this DUT has WiFi device.

[amd64]: https://en.wikipedia.org/wiki/X86-64
[run Android apps]: https://developer.android.com/topic/arc/
[Camera HAL3]: https://source.android.com/devices/camera/camera3
[AddressSanitizer]: https://github.com/google/sanitizers/wiki/AddressSanitizer
[Autotest capability]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-capability-default/
[chromeos-chrome]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/chromeos-chrome/chromeos-chrome-9999.ebuild
[Diagnostic utilities]: https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/diagnostics/README.md
[Downloadable Content]: https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/dlcservice
[video hardware overlays]: https://en.wikipedia.org/wiki/Hardware_overlay
[sandboxed]: https://chromium.googlesource.com/chromium/src/+/HEAD/docs/linux_sandboxing.md
[Memory stats collection daemon]: https://chromium.googlesource.com/chromiumos/platform2/+/master/metrics/memd/
[OCI]: https://www.opencontainers.org/
[screenshot command]: https://chromium.googlesource.com/chromiumos/platform2/+/master/screenshot/
[Trusted Platform Module]: https://en.wikipedia.org/wiki/Trusted_Platform_Module
[Transparent Hugepage]: https://www.kernel.org/doc/Documentation/vm/transhuge.txt
[run virtual machines]: https://chromium.googlesource.com/chromiumos/docs/+/master/containers_and_vms.md
[Vulkan]: https://www.khronos.org/vulkan/
[wilco]: https://sites.google.com/corp/google.com/wilco/home

## New features

Features should be descriptive and precise. Consider a hypothetical test that
exercises authentication using a biometrics daemon that isn't present in system
images built to run on virtual machines. Instead of adding a `real_hardware` or
`non_vm` feature that is overly broad and will likely be interpreted as carrying
additional meaning beyond the original intent, add a `biometrics_daemon` feature
that precisely communicates the test's actual requirement.

Features are composed from USE flags, which are statically defined when the
system image is built. [local_test_runner] lists boolean expressions that are
used to generate features; for example, an imaginary feature named `hd_audio`
with the expression

```go
cras && (audio_chipset_a || audio_chipset_b) && !broken_headphone_jack
```

will be reported as available on systems where the `cras` USE flag is set,
either `audio_chipset_a` or `audio_chipset_b` is set, and
`broken_headphone_jack` is explicitly *not* set. Before a new USE flag can be
used in an expression, it must be added to `IUSE` in the [tast-use-flags]
package, and before a feature can be listed by a test, it must be registered in
`local_test_runner`. Please use [CQ-DEPEND] in your commit messages to ensure
that changes land in the correct order.

If you're having trouble finding a way to specify your test's dependencies,
please ask for help on the [tast-users mailing list].

[local_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/local_test_runner/main.go
[tast-use-flags]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-use-flags/
[CQ-DEPEND]: https://chromium.googlesource.com/chromiumos/docs/+/master/contributing.md#cq-depend
[tast-users mailing list]: https://groups.google.com/a/chromium.org/forum/#!forum/tast-users

### Example changes

See the following changes for an example of adding a new `containers` software
feature based on the `containers` USE flag and making a test depend on it:

*   `chromiumos-overlay` repository: <https://crrev.com/c/1382877>
*   `tast` repository: <https://crrev.com/c/1382621>
*   `tast-tests` repository: <https://crrev.com/c/1382878>

(Note that the `containers` feature has since been renamed to `oci`.)

## autotest-capability

There are also `autotest-capability:`-prefixed features, which are added by the
[autocaps package] as specified by YAML files in
`/usr/local/etc/autotest-capability`. This exists in order to support porting
existing Autotest-based video tests to Tast. Do not depend on capabilities from
outside of video tests.

[autocaps package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/autocaps/
