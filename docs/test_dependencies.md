# Tast Test Dependencies (go/tast-deps)

A test may specify software features that must be supported by the DUT's system
image in order for the test to run successfully. If one or more features aren't
supported by the DUT, the test will (usually) be skipped. See the `tast`
command's `-checktestdeps` flag to control this behavior.

Tests specify dependencies through the `SoftwareDeps` field in [testing.Test].
The following software features are defined:

*   `android` - The ability to [run Android apps]. Any version of Android can be
    used.
*   `android_p` - The ability to [run Android apps] that require Android P or
    later.
*   `aslr` - Address space layout randomization, which mitigates buffer-overflow
    attacks, is functional (this is not true for builds with [AddressSanitizer]
    instrumentation built in).
*   `audio_play` - The ability to play audio.
*   `audio_record` - The ability to record audio.
*   `autotest-capability:foo` - An [Autotest capability] named `foo`. See below.
*   `camera_720p` - The ability to capture video with frame size 1280x720.
*   `chrome` - A Chrome process.
*   `chrome_internal` - Features that are only available in official Chrome,
    rather than Chromium. (e.g. proprietary codec like H.264)
*   `chrome_login` - Implies `chrome` with the further requirement that user
    login (i.e. using `session_manager` and `cryptohome`) is supported.
*   `cups` - CUPS daemon.
*   `display_backlight` - An internal display backlight.
*   `gpu_sandboxing` - Chrome's GPU process is [sandboxed].
*   `memd` - [Memory stats collection daemon].
*   `ml_service` - ML Service daemon.
*   `no_android` - The inability to run Android apps. This is the opposite of
    the `android` feature; DUTs will have exactly one of these two features.
*   `no_symlink_mount` - Symlink mounting is disabled via the
    `CONFIG_SECURITY_CHROMIUMOS_NO_SYMLINK_MOUNT` kernel option.
*   `reboot` - The ability to reboot reliably during a remote test.
*   `screenshot` - The [screenshot command] can save screenshots.
*   `selinux` - An SELinux-enabled board. All Android boards are
    SELinux-enabled.
*   `tpm` - A [Trusted Platform Module] chip.
*   `vm_host` - The ability to [run virtual machines].

Software features are composed from USE flags. [local_test_runner] lists boolean
expressions that are used to generate features; for example, an imaginary
feature named `hd_audio` with expression `cras && (audio_chipset_a ||
audio_chipset_b) && !broken_headphone_jack` will be reported as available on
systems where the `cras` USE flag is set, either `audio_chipset_a` or
`audio_chipset_b` is set, and `broken_headphone_jack` is explicitly *not* set.
Before a new USE flag can be used in an expression, it must be added to `IUSE`
in the [tast-use-flags] package.

The exception to the above are `autotest-capability:`-prefixed features, which
are added by the [autocaps package] as specified by YAML files in
`/usr/local/etc/autotest-capability`. This exists in order to support porting
existing Autotest-based video tests to Tast. Do not depend on capabilities from
outside of video tests.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[run Android apps]: https://developer.android.com/topic/arc/
[AddressSanitizer]: https://github.com/google/sanitizers/wiki/AddressSanitizer
[Autotest capability]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-capability-default/
[sandboxed]: https://chromium.googlesource.com/chromium/src/+/HEAD/docs/linux_sandboxing.md
[Memory stats collection daemon]: https://chromium.googlesource.com/chromiumos/platform2/+/master/metrics/memd/
[screenshot command]: https://chromium.googlesource.com/chromiumos/platform2/+/master/screenshot/
[Trusted Platform Module]: https://en.wikipedia.org/wiki/Trusted_Platform_Module
[run virtual machines]: https://chromium.googlesource.com/chromiumos/docs/+/master/containers_and_vms.md
[local_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/local_test_runner/main.go
[tast-use-flags]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-use-flags/
[autocaps package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/autocaps/
