# Tast: Running Tests (go/tast-running)

[TOC]

## Basic syntax

Tests can be executed within a ChromeOS chroot using the `tast` executable's
`run` command:

```shell
tast run <target> <test-pattern> <test-pattern> ...
```

To run private tests (e.g. `crosint` test bundle), use
`-buildbundle=<bundle-name>`.

Tests can also be run within a chrome-sdk using the `cros_run_test`
command:

```shell
third_party/chromite/bin/cros_run_test --device=<target> --tast <test-pattern>
```

When running `tast run` from a ChromeOS chroot, it compiles the tast binary in
`/platform/tast` and all tests in `/platform/tast-tests` and runs the code you
have checked out with any local changes.

When running `cros_run_test` from a chrome-sdk, the precompiled tests which
are packaged in the downloaded bundle will run which will be from the platform
version shown in chrome-sdk.

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
    See [go.chromium.org/tast/core/internal/expr] for details about expression syntax.
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

[go.chromium.org/tast/core/internal/expr]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/internal/expr
[Test Attributes]: test_attributes.md
[software dependencies]: test_dependencies.md

## Note for Chrome related tests

When you are running Tast tests that require Chrome, you should double check
that no unexpected arguments are specified in `/etc/chrome_dev.conf` on the DUT.
It can change Chrome's behavior and some tests might fail unexpectedly.

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
[Go in ChromiumOS]: https://www.chromium.org/chromium-os/developer-guide/go-in-chromium-os

## Running tests with Servo

Some tests use servo, a physical device that connects to both the host machine
and the DUT. These tests all specify `servo` as a [runtime variable], so they
must be run with that variable specifying the servo host and servo port.

If you can run Tast without [port forwarding], please use following syntax.

```shell
tast run -var=servo=<servo-host>:<servo-port> <target> <test-pattern>
```

If you need run Tast with [port forwarding], please use following syntax.

```shell
tast run -var=servo=localhost:<servo-port>:ssh:<servo_localhost_port> localhost:<DUT_localhost_port> <test-pattern>
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

In automated testing in the ChromeOS lab, Tast tests can reach a working Servo
device via `servo` runtime variable if they are scheduled with Autotest control
files declaring a Servo dependency. Control files for mainline tests declare it,
but other control files may not. See [crrev.com/c/2790771] for an example to add
a dependency declaration.

[runtime variable]: writing_tests.md#runtime-variables
[crrev.com/c/2790771]: https://crrev.com/c/2790771
[port forwarding]: running_tests.md#Option-2_Use-SSH-port-forwarding


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
[run.TestResult]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/cmd/tast/internal/run#TestResult
[JSONL]: http://jsonlines.org/
[output files]: writing_tests.md#Output-files
[perf]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/perf
[timing]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/timing

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

### Option 1: Use corp-ssh-helper-helper

In a window outside the chroot do,

*   Run gcert once a day.
*   Install [corp-ssh-helper-helper] and start the corp-ssh-helper-helper server process.

In another window inside chroot:

```shell
tast run <target> <test>
```

[corp-ssh-helper-helper]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/dev/contrib/corp-ssh-helper-helper/README.md

### Option 2: Use SSH port forwarding

In a window outside the chroot do,

*   Run gcert once a day.
*   Use [SSH Watcher] to port-forward a ssh connection to your device.
*   Use [SSH Watcher] to port-forward a ssh connection to labstation if you need
    to use servo.


Any port is fine as long as it is not used by other applications. Leave the SSH Watcher session(s) on.

In another window inside chroot:

Without servo port

```shell
tast run localhost:<port> <test>
```

With servo port

```
tast run --var=servo=localhost:${SERVO_PORT?}:ssh:${LOCAL_SERVO_SSH_PORT?} localhost:${LOCAL_DUT_SSH_PORT?} firmware.Fixture.normal
```

[SSH Watcher]: https://chromium.googlesource.com/chromiumos/platform/dev-util/+/HEAD/contrib/sshwatcher/README.md

## Running tests attached to a debugger
See [Tast Debugger](debugger.md)

## Bisecting tests broken by chromium
When a tast test fails on chromium CQ for an LKGM update, it means that some
change in chromium has broken the test.

Bisecting the failure is a little bit complicated since you have to repeatedly
compile chromium and deploy to a VM or DUT and then run a tast test from a cros
chroot.

This process can be made easier using `git bisect run` with a script.

You should be able to see the first bot that fails and find a good and bad chromium commit
via the BLAMELIST tab of the builder page.

### Example bisect.sh script
This script assumes there is a local betty VM running and that tests are
run from a ChromeOS chroot.
```shell
#!/bin/sh
set -x
gclient sync
autoninja -C out_betty/Release chrome
third_party/chromite/bin/deploy_chrome --board=betty --build-dir=out_betty/Release --device=ssh://localhost:9222 --verbose --strip-flags=-S --mount
cd ~/chromiumos
cros_sdk -- tast run -failfortests localhost:9222 terminal.Crosh
```

### chromium repo
```shell
git bisect good <last-commit-in-previous-good-build>
git bisect bad <last-commit-in-first-bad-build>
git bisect run ./bisect.sh
```

### Running Tast on a VM
There are a number of ChromeOS boards designed to run as VMs such as
`amd64-generic`, and `betty`.  Update `chromium/.gclient` file and add
`cros_boards` field in `custom_vars`:
```shell
solutions = [
  {
    ...
    "custom_vars" : {
        ...
        "cros_boards": "betty", # colon-separated list.
    },
  },
]
```

Download the SDK for the newly added board:
```shell
gclient sync
```

Enter chrome-sdk and download VM:
```shell
cros chrome-sdk --board=betty --download-vm --log-level=info --nogn-gen
```

Start the VM:
```shell
cros vm --start
```

You can view the screen output using VNC:
```shell
vncviewer localhost:5900 &
```

You may need to install a VNC viewer package:
```shell
sudo apt install xtightvncviewer
```

If you are going to compile and deploy chrome, you may want to update
`args.gn` with values to match what the bots use.  E.g.:
```shell
$ cat out_betty/Release/args.gn
import("//build/args/chromeos/betty.gni")
# Place any additional args or overrides below:
dcheck_always_on = true
exclude_unwind_tables = false
is_chrome_branded = true
use_remoteexec = true
```

Compile and deploy:
```shell
autoninja -C out_betty/Release chrome
third_party/chromite/bin/deploy_chrome --board=betty --build-dir=out_betty/Release --device=ssh://localhost:9222 --verbose --strip-flags=-S --mount
```

Run a test:
```shell
third_party/chromite/bin/cros_run_test --device=localhost:9222 --tast terminal.Crosh
```

### Download specific version of VM
If you need to use a different VM version than the current
`//chromeos/CHROMEOS_LKGM` used by chrome-sdk, you can download VMs directly
from google cloud storage. Open
https://ci.chromium.org/ui/p/chromeos/builders/postsubmit/betty-snapshot
, select a green build, and find `gs://` url for image. You can edit the URL and
replace `betty-snapshot` with any other board name to find the builds for other
boards.

```shell
cros flash --debug file:///tmp xbuddy://remote/betty-snapshot/R139-16318.0.0-110978-8712147517552604849
cros vm --start --board=betty --image-path /tmp/chromiumos_test_image.bin
```
