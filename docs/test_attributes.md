# Tast Test Attributes

Tests may specify attributes via the `Attr` field in [testing.Test]. Attributes
are free-form strings, but this document enumerates well-known attributes with
established meanings.

*   `arc` - Test exercises (and requires) [ARC], i.e. Android.
*   `bvt` - [Build Verification Test]. A failure justifies rejecting the
    responsible change.
*   `chrome` - Test exercises Chrome. A failure indicates a problem either in
    Chrome itself or in Chrome's integration into the OS.
*   `flaky` - Test is known to be flaky. It will still run, but failures will be
    not cause the suite to fail.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[ARC]: https://developer.android.com/topic/arc/index.html
[Build Verification Test]: https://en.wikipedia.org/wiki/Smoke_testing_(software)
