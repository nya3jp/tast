# Tast Test Dependencies

A test may specify software features that must be supported by the DUT's system
image in order for the test to run successfully. If one or more features aren't
supported by the DUT, the test will (usually) be skipped. See the `tast`
command's `-checktestdeps` flag to control this behavior.

Tests specify dependencies through the `SoftwareDeps` field in [testing.Test].
The following software features are defined:

*   `android` - The ability to run Android apps.
*   `chrome` - A Chrome process.
*   `chrome_login` - Implies `chrome` with the further requirement that user
    login (i.e. using `session_manager` and `cryptohome`) is supported.

Software features are composed from USE flags. [local_test_runner] lists boolean
expressions that are used to generate features; for example, an imaginary
feature named `hd_audio` with expression `cras && (audio_chipset_a ||
audio_chipset_b) && !broken_headphone_jack` will be reported as available on
systems where the `cras` USE flag is set, either `audio_chipset_a` or
`audio_chipset_b` is set, and `broken_headphone_jack` is explicitly *not* set.
Before a new USE flag can be used in an expression, it must be added to `IUSE`
in the [tast-use-flags] package.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[local_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/local_test_runner/main.go
[tast-use-flags]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-use-flags/
