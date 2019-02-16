# Modifying Tast

This document describes how to make changes to the Tast framework itself. For
information about writing Tast tests, see [Writing Tests].

[Writing Tests]: writing_tests.md

[TOC]

## Components

See the [Overview] document for a high-level description of the key components
and terminology in Tast.

[Overview]: overview.md

### Executables

*   The [tast executable] is run by developers and by builders. It's used to
    compile, deploy, and run tests and to collect results.
*   [local_test_runner] is run on the DUT by the `tast` process over an SSH
    connection. It collects system information and initiates running local
    tests.
*   [remote_test_runner] is run on the host system by the `tast` process to
    initiate running remote tests.

### Libraries

Shared library packages are located under the [tast directory]. Several packages
are particularly important:

*   [bundle] contains code used to implement test bundles, which contain
    compiled tests and are executed by test runners.
*   [control] defines control messages that are used for communication between
    the `tast` process, test runners, and test bundles.
*   [host] opens SSH connections.
*   [runner] contains code shared between `local_test_runner` and
    `remote_test_runner`.
*   [testing] contains code used to define and run tests.

[tast executable]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/tast/
[local_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/local_test_runner/
[remote_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/cmd/remote_test_runner/
[tast directory]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/
[bundle]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/bundle/
[control]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/control/
[host]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/host/
[runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/runner/
[testing]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/testing/

## Compiling changes

### fast_build.sh

The quickest way to rebuild the `tast` executable after modifying its code is by
running the [fast_build.sh] script located at the top of the `src/platform/tast`
repository within the Chrome OS chroot. This script bypasses Portage and runs
`go build` directly, allowing it to take advantage of [Go's build cache]. Since
dependency checks are skipped, there's no guarantee that the resulting
executable is correct – before uploading a change, you should verify it that it
builds when you run `FEATURES=test sudo emerge tast-cmd` (after running
`cros_workon --host start tast-cmd`).

*   Without any arguments, `fast_build.sh` compiles the `tast` executable to
    `$HOME/bin/tast`.
*   `fast_build.sh -t chromiumos/tast/testing` runs the unit tests for the
    `chromiumos/tast/testing` package.
*   `fast_build.sh -T` runs all unit tests (including ones in the `tast-tests`
    repository).
*   `fast_build.sh -c chromiumos/tast/testing` runs `go vet` against the
    `chromiumos/tast/testing` package.
*   `fast_build.sh -C` vets all packages.

Run `fast_build.sh -h` to see all available options.

[fast_build.sh]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/fast_build.sh
[Go's build cache]: https://golang.org/cmd/go/#hdr-Build_and_test_caching

## Testing changes

### Unit tests

The different components of the framework are extensively covered by unit tests.
Please ensure that any changes that you make are also covered by tests.

The [testutil package] provides utility functions intended to reduce repetitive
code within unit tests.

[testutil package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testutil

### Meta tests

There are several [meta tests]. These are remote Tast tests that run a nested
instance of the `tast` executable to perform end-to-end verification of
interactions between `tast`, test runners, and test bundles. They're executed
the same way as other Tast tests, i.e. via `tast run`.

[meta tests]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/bundles/cros/meta/
