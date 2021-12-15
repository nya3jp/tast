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

[tast executable]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/cmd/tast/
[local_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/cmd/local_test_runner/
[remote_test_runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/cmd/remote_test_runner/

### Libraries

Shared library packages are located under the [tast directory]. Several packages
are particularly important:

*   [bundle] contains code used to implement test bundles, which contain
    compiled tests and are executed by test runners.
*   [protocol] defines protocol buffer messages that are used for communication
    between the `tast` process, test runners, and test bundles.
*   [runner] contains code shared between `local_test_runner` and
    `remote_test_runner`.
*   [ssh] opens SSH connections.
*   [testing] contains code used to define and run tests.

[tast directory]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/
[bundle]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/bundle/
[protocol]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/internal/protocol/
[runner]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/internal/runner/
[ssh]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/ssh/
[testing]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/testing/

## Making changes

### IPC

JSON-marshaled structs are used for communication between processes:

*   The [tast executable]'s [run] package marshals and passes [runner.Args] to
    test runners.
*   [local_test_runner] and [remote_test_runner] use the [runner] package to
    unmarshal [runner.Args] and marshal and pass [bundle.Args] structs to test
    bundles.
*   Test bundles use the [bundle] package to unmarshal [bundle.Args].

The [runner.Args] and [bundle.Args] structs contain other `Args`-suffixed
structs that may or may not be set depending on the type of request that is
being made. Note also that [runner.RunTestsArgs] includes [bundle.RunTestsArgs]
and that [runner.ListTestsArgs] includes [bundle.ListTestsArgs].

When running in the Chrome OS hardware lab or on VM builders, matching versions
of `tast`, test runners, and test bundles are used, so there are no
compatibility concerns.

More combinations are possible when running in a developer's chroot:

*   The `tast` binary in the chroot can be updated by emerging `tast-cmd` or
    running `fast_build.sh`.
*   The `remote_test_runner` binary in the chroot is typically updated by
    emerging `tast-cmd`.
*   The `local_test_runner` binary on the DUT can either be installed by the
    `tast-local-test-runner` package or updated by running `tast run
    -build=true`.
*   The default test bundle on the DUT can be installed by the
    `tast-local-tests-cros` package or updated by running `tast run
    -build=true`.

As a result, developers may end up running the `tast` executable in their chroot
against DUTs that contain either older or newer versions of `local_test_runner`
and test bundles. To maintain compatibility, please ensure that the following
conditions hold across both the `tast`-to-runner and runner-to-bundle
boundaries:

*   Older receivers correctly handle structs marshaled by newer senders.
*   Newer receivers correctly handle structs marshaled by older senders.

Go's [json package] does best-effort unmarshaling, ignoring unknown fields by
default. When renaming or moving a field, it's advisable to preserve the old
field for at least two months. Add a `Deprecated` suffix to the old field's name
while preserving its original marshaled name in its `json` tag.

`tast` and the [runner] package should set both the old and new fields, and the
[runner] and [bundle] packages should copy the old fields to the new fields when
the former are provided. See [change 1474620] for an example.

[run]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/chromiumos/tast/cmd/tast/internal/run/
[runner.Args]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/internal/runner#Args
[bundle.Args]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/bundle#Args
[runner.RunTestsArgs]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/internal/runner#RunTestsArgs
[bundle.RunTestsArgs]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/bundle#RunTestsArgs
[runner.ListTestsArgs]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/internal/runner#ListTestsArgs
[bundle.ListTestsArgs]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/bundle#ListTestsArgs
[json package]: https://golang.org/pkg/encoding/json/
[change 1474620]: https://crrev.com/c/1474620

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
    `$HOME/go/bin/tast`.
*   `fast_build.sh -t chromiumos/tast/testing` runs the unit tests for the
    `chromiumos/tast/testing` package.
*   `fast_build.sh -T` runs all unit tests (including ones in the `tast-tests`
    repository).
*   `fast_build.sh -c chromiumos/tast/testing` runs `go vet` against the
    `chromiumos/tast/testing` package.
*   `fast_build.sh -C` vets all packages.

Run `fast_build.sh -h` to see all available options.

[fast_build.sh]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/fast_build.sh
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
