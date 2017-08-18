# Tast Overview

## Terminology

### Local tests

Local tests run directly on devices-under-test (DUTs) and are similar in
function to Autotest's "client" tests. They can do anything that any other code
running locally as root can do. Most tests should be local tests.

### Remote tests

Remote tests run on the machine initiating testing (e.g. a developer's
workstation) and run shell commands on the DUT remotely, allowing the DUT to be
rebooted in the course of a single test. They are similar in function to
Autotest's "server" tests. They run more slowly than local tests, are harder to
write, and are more susceptible to infrastructure and hardware issues.

### `tast` command

The `tast` executable is built for the host system by the
`chromeos-base/tast-cmd` package. The developer runs it within their chroot to:

*   compile test executables for a given target architecture (the DUT's
    architecture for local tests or the initiating machine's architecture for
    remote tests),
*   push tests and data files to DUTs (only in the case of local tests) and run
    tests, and
*   collect, process, and display test results.

### Test executables

Test executables (`local_tests` and `remote_tests`) consist of compiled tests,
along with code for running them and reporting results back to `tast`. Local
tests can be included in a system image via the `chromeos-base/tast-local-tests`
package, while remote tests can be installed onto the host system via the
`chromeos-base/tast-remote-tests` package. Either type of test can also be
compiled (and deployed, in the case of local tests) on-the-fly by `tast`.

Tests don't (currently) require anything special from the OS -- `tast` just
needs SSH access to the DUT and (when deploying local tests that aren't built
into the system image) a writable partition that isn't mounted `noexec`.

## Process

Tests can be cross-compiled within the chroot by the `tast` executable using the
Go toolchain. Object files are cached and reused where possible -- caching
happens at the package level, and tests are grouped into packages by category,
e.g. `ui`, `power`, `security`, etc.

For local tests, `tast` establishes a single SSH connection to the DUT for all
communication. The connection is used to copy `local_tests` and associated test
data files to the DUT (if not built into the system image), execute
`local_tests`, and receive the results (including system logs or output data
written by the tests).

For remote tests, tast runs `remote_tests` directly and receives results
provided by it. `remote_tests` establishes SSH connections to the DUT and passes
them to tests.
