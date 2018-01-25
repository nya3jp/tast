# Tast Test Attributes

Tests may specify attributes via the `Attr` field in [testing.Test]. Attributes
are free-form strings, but this document enumerates well-known attributes with
established meanings.

See the [Running Tests] document for information about using attributes to
select which tests to run.

*   `arc` - Test exercises (and requires) [ARC], i.e. Android.
*   `bundle:<bundle>` Test's bundle, e.g. `cros` (automatically added).
*   `bvt` - [Build Verification Test]. A failure justifies rejecting the
    responsible change.
*   `chrome` - Test exercises Chrome. A failure indicates a problem either in
    Chrome itself or in Chrome's integration into the OS.
*   `flaky` - Test is known to be flaky. It will still run, but failures will be
    not cause the suite to fail.
*   `name:<category.Test>` - Test's full name (automatically added).

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[ARC]: https://developer.android.com/topic/arc/index.html
[Build Verification Test]: https://en.wikipedia.org/wiki/Smoke_testing_(software)
[Running Tests]: running_tests.md
