# Tast Test Attributes (go/tast-attr)

Tests may specify attributes via the `Attr` field in [testing.Test]. Attributes
are free-form strings, but this document enumerates well-known attributes with
established meanings.

## Manually-added attributes

The following attributes may be added to control how tests are run and how their
results are interpreted:

*   `disabled` - Test is not run automatically in the lab.
*   `informational` - Test failures are ignored.

Failures in tests without `disabled` or `informational` attributes justify
rejecting or reverting the responsible change.

## Automatically-added attributes

Several attributes are also added automatically:

*   `bundle:<bundle>` - Test's [bundle], e.g. `cros` (automatically added).
*   `dep:<dependency>` - Test [software dependency] (automatically added).
*   `name:<category.Test>` - Test's full [name] (automatically added).

## Using attributes

See the [Running Tests] document for information about using attributes to
select which tests to run.

[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[bundle]: overview.md#Test-bundles
[software dependency]: test_dependencies.md
[name]: writing_tests.md#Test-names
[Running Tests]: running_tests.md
