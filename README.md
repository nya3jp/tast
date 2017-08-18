# Tast

Tast is an integration-testing system for Chrome OS. Its focus is on
maintainability, speed, and ease of interpreting and reproducing test results.
It supports building, deploying, and running tests. It doesn't implement other
functionality like managing labs of devices used for testing, scheduling tests,
or storing test results.

The [overview] is a good starting point.

## Directory structure

This repository is organized in accordance with the [Go in Chromium OS]
suggestions.

*   [`src/chromiumos/tast/`](src/chromiumos/tast/)
    *   [`common/`](src/chromiumos/tast/common/) - Packages shared between two
        or more of `local/`, `remote/`, and `tast/`.
    *   [`local/`](src/chromiumos/tast/local/) - `main` package for the
        `local_tests` executable containing "local" tests, i.e. ones that run
        on-device.
        *   [`tests/`](src/chromiumos/tast/local/tests) - Local tests, packaged
            by category.
        *   `...` - Packages used only by local tests.
    *   [`remote/`](src/chromiumos/tast/remote/) - `main` package for the
        `remote_tests` executable containing "remote" tests, i.e. ones that run
        off-device.
        *   [`tests/`](src/chromiumos/tast/remote/tests/) - Remote tests,
            packaged by category.
        *   `...` - Packages used only by remote tests.
    *   [`tast/`](src/chromiumos/tast/tast) - `main` package for the `tast`
        executable used to build and run tests.
        *   `...` - Packages used only by the `tast` executable.

## Documentation

For more details, see the [docs](docs/) subdirectory.

[overview]: docs/overview.md
[Go in Chromium OS]: http://www.chromium.org/chromium-os/developer-guide/go-in-chromium-os
