# Tast Test Attributes (go/tast-attr)

Tests may specify attributes via the `Attr` field in [testing.Test]. Attributes
are free-form strings, but this document enumerates well-known attributes with
established meanings.

## Manually-added attributes

The following attributes may be added to control how tests are run and how their
results are interpreted.

Zero or more `group:*` attributes can be set to a test to assign it to
*groups*. A group is a collection of tests having similar purposes or
characteristics, often (but not necessarily) scheduled together.
In automated testing, tests to run are usually selected by groups.
If a test belongs to no group, it has no attribute `group:*` assigned, that
means it will be [disabled] in automated testing.

Some groups define *extra attributes* which annotate tests with extra
information. They can be set if a test belongs to corresponding groups.

Below is the list of most popular groups and their associated extra attributes:

*   `group:mainline` - The default group for functional tests. Tests having
    this attribute are called *mainline* tests. A mainline test falls under
    exactly one of the two categories: *informational* if it has `informational`
    attribute; otherwise it is *critical*.
    Failures in critical tests justify rejecting or reverting the responsible
    change, while failures in informational tests are ignored.
    All informational mainline tests are supposed to be promoted to critical
    tests. Details on adding tests can be found in this [tast-add-test] document (googlers only).
     * `group:criticalstaging` - This group will be used to indicate a test is intended on
       going into "mainline" critical testing. This group will be run on all boards/models;
       on ToT only.  Tests can only remain in this group long enough to gather signal (10 consecutive builds),
       after which the owner must promote them into mainline only, or back into
       informational. If no owner action is taken after a 4 builds grace period, they
       will be moved into informational.
     * `informational`: This group indicate a test is not critical.
*   `group:hw_agnostic` - A test that is not dependent on physical HW and can be run on x86 VMs.
     * `vm_stable`: This indicates that a test is stable on VMs. It is intended for
        mainline informational tests that have yet to be stabilized on all boards/models, but
        is safe and stable on VMs. This enables such tests to be scheduled and tested on VM only.
*   `group:appcompat` - A group of ARC app compatibility tests.
    Below are its sub-attribute:
     * `appcompat_release`: A group of ARC app compatibility tests for release testing.
     * `appcompat_smoke`: A group of ARC app compatibility tests for smoke testing.
     * `appcompat_top_apps`: A group of ARC app compatibility tests for top apps testing.
*   `group:arc` - A group of ARC Functional tests.
    Below are its sub-attribute:
     * `arc_playstore`: A group of ARC tests which verify playstore
     functionality after updating the playstore to nightly build.
     * `arc_core`: A group of ARC tests which verify ARC Core functionality.
     * `arc_chromeos_vm`: A group of ARC tests which run ARC functionality on ChromeOS VM.
*   `group:crosbolt` - Test failures are ignored and the test's performance data
    are uploaded to [crosbolt]. When you add this attribute, you also need to
    add one of `crosbolt_perbuild`, `crosbolt_nightly` or `crosbolt_weekly`.
*   `group:wificell` - Tests that will run on the [wificell] fixture. Typically
    those tests require special hardware (APs, ...) only available in those
    fixtures. Some WiFi-specific functional tests that do not technically
    require a [wificell] are also made part of this group for consistency to
    simplify the validation of the whole WiFi stack.
    Its sub-attributes can be classified into two types:
    *  Role (required): specify the role of the test:
       *  `wificell_func`: verify basic WiFi functionalities nightly.
       *  `wificell_suspend`: verify basic WiFi behavior related to
          suspend/resume nightly.
       *  `wificell_cq`: Similar to wificell_func, but triggered by CLs that
          touch specific code paths.
       *  `wificell_perf`: measure WiFi performance.
       *  `wificell_stress`: Stress test the WiFi functionalities.
       *  `wificell_mtbf`: measure Mean Time Between Failures (MTBF).
    *  Stability (optional): if `wificell_unstable` is present, the test is yet
       to be verified as stable; otherwise, the test is stable.
*   `group:wificell_roam` - Tests that depends on [grover] (Googlers only) fixture to run.
    Subattributes that specify role of the test (required):
    *  `wificell_roam_func`: verify basic WiFi roaming functionalities.
    *  `wificell_roam_perf`: measure WiFi roaming performance.
*   `group:labqual` - Tests that must pass for devices to go to a low-touch lab.
*   `group:mtp` - Tests that will run on the `mtpWithAndroid` fixture. Typically
    those tests require special hardware (Android Phone Connected to DUT) and setup
    available in the fixture.
*   `group:cuj` - A group of CUJ tests. Tests having this attribute will have their
    performance data collected and sent to TPS dashboard.
    Below are its sub-attributes:
    *  `cuj_experimental`: A group of CUJ tests which are experimental and only run
       on a selected subset of models.
    *  `cuj_weekly`: A group of CUJ tests that run weekly.
See [attr.go] for the full list of valid attributes.

## Automatically-added attributes

Several attributes are also added automatically:

*   `bundle:<bundle>` - Test's [bundle], e.g. `cros` (automatically added).
*   `dep:<dependency>` - Test [software dependency] (automatically added).
*   `name:<category.Test>` - Test's full [name] (automatically added).

## Using attributes

See the [Running Tests] document for information about using attributes to
select which tests to run.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Test
[crosbolt]: https://crosbolt.teams.x20web.corp.google.com/prod/crosbolt/index.html
[wificell]: https://chromium.googlesource.com/chromiumos/third_party/autotest/+/main/docs/wificell.md
[grover]: https://docs.google.com/document/d/1klnkcEpbG6_0BKeLXEN9ST13-w8gcNYG1xH80dlvt7U/edit# (Googlers only)
[attr.go]: https://chromium.googlesource.com/chromiumos/platform/tast/+/refs/heads/main/src/go.chromium.org/tast/core/internal/testing/attr.go
[bundle]: overview.md#Test-bundles
[software dependency]: test_dependencies.md
[name]: writing_tests.md#Test-names
[Running Tests]: running_tests.md
[disabled]: writing_tests.md#Disabling-tests
[tast-add-test]: http://go/tast-add-test
