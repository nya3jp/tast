# Tast: Running Tests (go/tast-running)

[TOC]

## Basic syntax

Tests can be executed within a Chrome OS chroot using the `tast` executable's
`run` command:

```shell
tast run <target> <test-pattern> <test-pattern> ...
```

## Specifying where to run tests

The first positional argument supplied to the `run` subcommand specifies the
"target", i.e. the device where the test will be run, also known as the
device-under-test or DUT. In the case of local tests, the test code will run
directly on the DUT. For remote tests, the test code will run on the host
machine but connect to the DUT. Expressions like `root@10.0.0.1:22`,
`root@localhost`, and `10.0.0.2` are supported. The root user is used by
default, as tests frequently require root access to the DUT.

By default, the standard `testing_rsa` key from `chromite/ssh_keys` will be used
to establish an SSH connection with the device. The `keyfile` flag can be
supplied to specify a different private key:

```shell
tast run -keyfile=$HOME/.ssh/id_rsa ...
```

## Specifying which tests to run

Any additional positional arguments describe which tests should be executed:

*   If no arguments are supplied, all tests are selected.
*   If a single argument surrounded by parentheses is supplied, it is
    interpreted as a boolean expression consisting of test attributes. For
    example, the expression `(("dep:chrome" || "dep:chrome_login") && !flaky &&
    !disabled)` matches all tests with a `dep:chrome` or `dep:chrome_login`
    attribute but not a `flaky` or `disabled` attribute. Attributes that don't
    consist of a letter or underscore followed by letters, digits, and
    underscores must be double-quoted. '*' characters in quoted strings are
    interpreted as wildcards. See [chromiumos/tast/expr] for details about
    expression syntax.
*   Otherwise, the argument(s) are interpreted as wildcard patterns matching
    test names. For example, `ui.*` matches all tests with names prefixed by
    `ui.`. Multiple patterns can be supplied: passing `example.Pass` and
    `example.Fail` selects those two tests.

It's invalid to mix attribute expressions and wildcard patterns. See the [Test
Attributes] document for more information about attributes.

Tests may be skipped if they list [software dependencies] that aren't provided
by the DUT. This behavior can be controlled via the `tast` command's
`-checktestdeps` flag.

If tests are being rebuilt, the `buildtype` flag determines which type of tests
to build and run; see the next section for details. Otherwise, both local and
remote tests are considered.

## Controlling whether tests are rebuilt

When the `-build` flag is true (the default), `tast run` rebuilds the `cros`
test bundle and (in the case of local tests) pushes it to the DUT as
`/usr/local/share/tast/bundles_pushed/cros`. This permits faster compilation and
deployment when writing new tests than the normal `emerge`/`cros deploy` cycle
can provide.

The `buildtype` flag accepts a `local` or `remote` parameter to specify which
type of tests to build, push, and run. `local` is the default.

The name of the bundle to build, push, and run can be specified via the
`-buildbundle` flag. If the bundle's source code is outside of the `tast-tests
repository`, you will need to specify the repository's path using the
`-buildtestdir` flag.

To rebuild a test bundle, the `tast` command needs its dependencies' source code
to be available. This code is automatically checked out to `/usr/lib/gopath`
when building packages for the host system, as described in the [Go in Chromium
OS] document. The `tast` command will automatically inform you when the bundle's
dependencies need to be manually emerged.

To skip rebuilding a bundle and instead run all builtin bundles within the
`/usr/local/share/tast/bundles` directory on the DUT (for local tests) and
`/usr/share/tast/bundles` on the host system (for remote tests), pass
`-build=false`. The default builtin `cros` local bundle will only be present if
the `chromeos-base/tast-local-tests-cros` package was emerged to the DUT. This
happens automatically when a `test` system image is built.

If `-build` is true and `local_test_runner` isn't present on the DUT (presumably
because it's running a `dev` system image rather than a `test` image), the
`tast` command will attempt to build and deploy it.

## Interpreting test results

As each test runs, its output is streamed to the `tast` executable. Overall
information about the current state of the test run is logged to stdout by
default. The top-level (i.e. `tast -verbose run ...`) `-verbose` flag can be
supplied to log additional information to the console, including all messages
written by tests.

By default, test results are written to a subdirectory under
`/tmp/tast/results`, but an alternate directory can be supplied via the `run`
command's `-resultsdir` flag. If the default directory is used, a symlink will
also be created to it at `/tmp/tast/results/latest`.

Various files and directories are created within the results directory:

*   `crashes/` - [Breakpad] minidump files with information about crashes that
    occured during testing.
*   `full.txt` - All output from the run, including messages logged by
    individual tests.
*   `results.json` - Machine-parseable test results.
*   `run_error.txt` - Error message describing the reason why the run was
    aborted (e.g. SSH connection to DUT was lost). Only written when a global
    error occurs.
*   `streamed_results.jsonl` - Streamed machine-parseable test results. Provides
    partial results if `tast` process is interrupted before `results.json` is
    written.
*   `system_logs/` - Diff of `/var/log` on the DUT before and after testing.
*   `tests/` - Per-test subdirectories, each of which has messages logged by the
    test and any output files that it created.
*   `timing.json` - Machine-parsable timing information about the test run.

## Running local tests manually on the DUT

If you need to run one or more local tests manually on a DUT (e.g. because you
don't have a Chrome OS chroot containing the `tast` executable), the
`local_test_runner` executable can be started directly on the DUT:

```shell
local_test_runner ui.ChromeLogin
```

[chromiumos/tast/expr]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/expr
[Test Attributes]: test_attributes.md
[software dependencies]: test_dependencies.md
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[Go in Chromium OS]: https://www.chromium.org/chromium-os/developer-guide/go-in-chromium-os
[Breakpad]: https://github.com/google/breakpad/
