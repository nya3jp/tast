# Tast: Running Tests (go/tast-running)

[TOC]

## Basic syntax

Tests can be executed within a Chrome OS chroot using the `tast` executable's
`run` command:

```shell
tast run <target> <test-pattern> <test-pattern> ...
```

To run private tests (e.g. `crosint` test bundle), use
`-buildbundle=<bundle-name>`.

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
    interpreted as a boolean expression consisting of test attributes.
    For example, the expression
    `(("dep:chrome" || "dep:android") && !informational)` matches all tests with
    a `dep:chrome` or `dep:android` attribute but not an `informational`
    attribute. Attributes that don't consist of a letter or underscore followed
    by letters, digits, and underscores must be double-quoted. '*' characters in
    quoted strings are interpreted as wildcards.
    See [chromiumos/tast/internal/expr] for details about expression syntax.
*   Otherwise, the argument(s) are interpreted as wildcard patterns matching
    test names. For example, `ui.*` matches all tests with names prefixed by
    `ui.`. Multiple patterns can be supplied: passing `example.Pass` and
    `example.Fail` selects those two tests.
*   It's invalid to mix attribute expressions and wildcard patterns. To use a 
    wildcard to match against the test name you can use the `"name:ui.*"`
    expression instead.

See the [Test Attributes] document for more information about attributes.

Tests may be skipped if they list [software dependencies] that aren't provided
by the DUT. This behavior can be controlled via the `tast` command's
`-checktestdeps` flag.

[chromiumos/tast/internal/expr]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/internal/expr
[Test Attributes]: test_attributes.md
[software dependencies]: test_dependencies.md

## Controlling whether tests are rebuilt

When the `-build` flag is true (the default), `tast run` rebuilds the `cros`
test bundle and pushes it to the DUT as
`/usr/local/share/tast/bundles_pushed/cros`. This permits faster compilation and
deployment when writing new tests than the normal `emerge`/`cros deploy` cycle
can provide.

The name of the bundle to build, push, and run can be specified via the
`-buildbundle` flag. If the bundle's source code is outside of the [tast-tests
repository], you will need to specify the repository's path using the
`-buildtestdir` flag.

To rebuild a test bundle, the `tast` command needs its dependencies' source code
to be available. This code is automatically checked out to `/usr/lib/gopath`
when building packages for the host system, as described in the [Go in Chromium
OS] document. The `tast` command will automatically inform you when the bundle's
dependencies need to be manually emerged.

To skip rebuilding a bundle and instead run all builtin bundles within the
`/usr/local/share/tast/bundles` directory on the DUT (for local tests) and
`/usr/share/tast/bundles` on the host system (for remote tests), pass
`-build=false`. The default builtin `cros` local bundle should be present on
all `test` system images (non-`test` system images are not supposed by Tast).

[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[Go in Chromium OS]: https://www.chromium.org/chromium-os/developer-guide/go-in-chromium-os

## Running tests with Servo

Some tests use servo, a physical device that connects to both the host machine
and the DUT. These tests all specify `servo` as a [runtime variable], so they
must be run with that variable specifying the servo host and servo port:

```shell
tast run -var=servo=<servo-host>:<servo-port> <target> <test-pattern>
```

In order for a test to interact with the servo, the servo host must be running
an instance of `servod` (servo daemon) on the appropriate port. When Tast is
run through the Tauto wrapper via `test_that`, Tauto takes care of initiating
and closing `servod`. However, when Tast is run through `tast run`, it does not
initiate `servod`; the user must initiate `servod` from the servo host:

```shell
ssh <servo-host>
servod --board=<board> --model=<model> --port=<servo-port> --serialname=<servo-serial>
```

In automated testing in the Chrome OS lab, Tast tests can reach a working Servo
device via `servo` runtime variable if they are scheduled with Autotest control
files declaring a Servo dependency. Control files for mainline tests declare it,
but other control files may not. See [crrev.com/c/2790771] for an example to add
a dependency declaration.

[runtime variable]: writing_tests.md#runtime-variables
[crrev.com/c/2790771]: https://crrev.com/c/2790771

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
*   `results.json` - Machine-parseable test results, supplied as a
    JSON-marshaled array of [run.TestResult] structs.
*   `run_error.txt` - Error message describing the reason why the run was
    aborted (e.g. SSH connection to DUT was lost). Only written when a global
    error occurs.
*   `streamed_results.jsonl` - Streamed machine-parseable test results, supplied
    as a [JSONL] array of [run.TestResult] structs. Provides partial results if
    the `tast` process is interrupted before `results.json` is written.
*   `system_logs/` - Diff of `/var/log` on the DUT before and after testing.
    *   `unified/` - Unified log collected from system logs.
        *   `unified.log` - Human-readable system log messages.
        *   `unified.export.gz` - gzip-compressed logs with full metadata from
            croslog's export mode which is similler to `journalctl -o export`.
*   `tests/<test-name>/` - Per-test subdirectories, containing test logs and
    other output files.
    *   `log.txt` - Log of messages and errors reported by the test.
    *   (optional) `results-chart.json` - Machine-parseable performance
        metrics produced by the [perf] package.
    *   `...` - Other [output files] from the test.
*   `timing.json` - Machine-parsable JSON-marshaled timing information about the
    test run produced by the [timing] package.

[Breakpad]: https://github.com/google/breakpad/
[run.TestResult]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/cmd/tast/internal/run#TestResult
[JSONL]: http://jsonlines.org/
[output files]: writing_tests.md#Output-files
[perf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/perf
[timing]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/timing

## Reset the device owner of the DUT after test run

Tast resets the device owner of the DUT before test run, and after the test run,
the device owner remains to be testuser. To reset that, run the following on the
DUT:

```shell
stop ui
rm -rf /var/lib/devicesettings '/home/chronos/Local State'
start ui
```
## Googlers Only: Running tests on a leased DUT from the lab

In a window outside the chroot do,

```shell
gcert  # Once a day
ssh -L <port>:localhost:22 root@<dut> # One session for each DUT
```

Any port is fine as long as it is not used by other applications. Leave the ssh session on.

In another window inside chroot:

```shell
tast run localhost:<port> <test>
```
