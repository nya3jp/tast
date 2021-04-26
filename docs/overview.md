# Tast Overview (go/tast-overview)

[TOC]

## Terminology

### Local tests

Local tests run directly on devices-under-test (DUTs) and are similar in
function to Autotest's "client" tests. They can do anything that any other code
running locally as root can do. Most tests should be local tests.

### Remote tests

Remote tests run on the machine initiating testing (e.g. a developer's
workstation, a.k.a. the "host system") and run shell commands on the DUT
remotely, allowing the DUT to be rebooted in the course of a single test. They
are similar in function to Autotest's "server" tests. They run more slowly than
local tests, are harder to write, and are more susceptible to infrastructure and
hardware issues.

### `tast` command

The `tast` executable is built for the host system by the
`chromeos-base/tast-cmd` Portage package from code in the [tast repository]. The
developer runs it within their chroot to:

*   compile test bundles for a given target architecture (the DUT's architecture
    for local tests or the initiating machine's architecture for remote tests),
*   push test bundles and data files to DUTs (only in the case of local tests)
    and run tests, and
*   collect, process, and display test results.

The [Running Tests] document contains more information about using the `tast`
command.

### Test bundles

Test bundles consist of executables and their associated data files (internal
data files + external data link files). Test bundle executables are composed of
compiled tests along with code for running them and reporting results back to
a test runner. Bundles exist so that test code can be checked into other
(probably non-public) repositories.

The default local and remote bundles, consisting of Chrome OS tests, are named
`cros` and are checked into the [tast-tests repository].

Local tests can be included in a system image via Portage packages with names of
the form `chromeos-base/tast-local-tests-<bundle>`, while remote tests can be
installed onto the host system via packages named
`chromeos-base/tast-remote-tests-<bundle>`. Either type of bundle can also be
compiled (and deployed, in the case of local tests) on-the-fly by `tast`.

Tests don't (currently) require anything special from the OS — `tast` just needs
SSH access to the DUT and (when deploying local test bundles that aren't built
into the system image) a writable partition that isn't mounted `noexec`.

See the [Writing Tests] document for more information.

### Test runners

Test runners are executables that enumerate and execute test bundles before
aggregating results for the `tast` command. There are two test runners,
`local_test_runner` (which executes local test bundles on-device) and
`remote_test_runner` (which runs remote test bundles on the host system).
Runners are installed by the `chromeos-base/tast-local-test-runner` and
`chromeos-base/tast-cmd` Portage packages, respectively, and are built from code
in the [tast repository].

## Process

Test bundles can be cross-compiled within the chroot by the `tast` executable
using the Go toolchain. Object files are cached and reused where possible —
caching happens at the package level, and the tests within each bundle are
grouped into packages by category, e.g. `ui`, `power`, `security`, etc.

For local tests, `tast` establishes a single SSH connection to the DUT for all
communication. The connection is used to copy a local test bundle and associated
test data files to the DUT (if not built into the system image), execute
`local_test_runner`, and receive the results (including system logs, crash
dumps, and output data written by the tests).

For remote tests, tast executes `remote_test_runner` directly on the host system
and receives results provided by it. Each remote test bundle establishes an SSH
connection to the DUT and passes it to tests.

The [Design Principles] document describes the thinking that guided Tast's
design.

[tast repository]: https://chromium.googlesource.com/chromiumos/platform/tast/
[Running Tests]: running_tests.md
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[Writing Tests]: writing_tests.md
[Design Principles]: design_principles.md
