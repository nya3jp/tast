# Tast Test Attributes (go/tast-attr)

Tests may specify attributes via the `Attr` field in [testing.Test]. Attributes
are free-form strings, but this document enumerates well-known attributes with
established meanings.

## Manually-added attributes

The following attributes may be added to control how tests are run and how their
results are interpreted.

A test should have at least one or more `group:*` attribute or `disabled`
attribute. Some additional attributes can be specified depending on `group:*`
attributes.

*   `group:mainline` - The default group for functional tests. Tests having
    this attribute are called *mainline* tests.
    *   `informational` - Test failures are ignored. Tests **not** having this
        attribute are called *critical* tests. All informational mainline tests
        are supposed to be promoted to critical tests. Failures in critical
        tests justify rejecting or reverting the responsible change.
*   `group:crosbolt` - Test failures are ignored and the test's performance data
    are uploaded to [crosbolt]. When you add this attribute, you also need to
    add one of `crosbolt_perbuild`, `crosbolt_nightly` or `crosbolt_weekly`.
*   `disabled` - Test is not run automatically in the lab.

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
[bundle]: overview.md#Test-bundles
[software dependency]: test_dependencies.md
[name]: writing_tests.md#Test-names
[Running Tests]: running_tests.md
