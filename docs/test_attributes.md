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
This means that a test will not run in automated testing if it belongs
to no group.

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
    tests.
*   `group:crosbolt` - Test failures are ignored and the test's performance data
    are uploaded to [crosbolt]. When you add this attribute, you also need to
    add one of `crosbolt_perbuild`, `crosbolt_nightly` or `crosbolt_weekly`.
*   `group:wificell` - Tests that depends on [wificell] fixture to run.
    Currently it has only one sub-attribute: `wificell_func`, which is used to
    verify basic WiFi functionalities.

See [attr.go] for the full list of valid attributes.

## Automatically-added attributes

Several attributes are also added automatically:

*   `bundle:<bundle>` - Test's [bundle], e.g. `cros` (automatically added).
*   `dep:<dependency>` - Test [software dependency] (automatically added).
*   `name:<category.Test>` - Test's full [name] (automatically added).

## Using attributes

See the [Running Tests] document for information about using attributes to
select which tests to run.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[crosbolt]: https://crosbolt.teams.x20web.corp.google.com/prod/crosbolt/index.html
[wificell]: https://chromium.googlesource.com/chromiumos/third_party/autotest/+/master/docs/wificell.md
[attr.go]: https://chromium.googlesource.com/chromiumos/platform/tast/+/refs/heads/master/src/chromiumos/tast/testing/attr.go
[bundle]: overview.md#Test-bundles
[software dependency]: test_dependencies.md
[name]: writing_tests.md#Test-names
[Running Tests]: running_tests.md
