# Tast

Tast is an integration-testing system for Chrome OS. Its focus is on
maintainability, speed, and ease of interpreting and reproducing test results.
It supports building, deploying, and running tests. It doesn't implement other
functionality like managing labs of devices used for testing, scheduling tests,
or storing test results.

To try it out, see the [quickstart] document. The [overview] is a good starting
point for learning more about how the system is structured.

## Directory structure

This repository is organized in accordance with the [Go in Chromium OS]
suggestions.

*   [`src/chromiumos/tast/`](src/chromiumos/tast/)
    *   [`cmd/`](src/chromiumos/tast/cmd/) - `main` package for the `tast`
        executable used to build and run tests.
        *   `...` - Packages used only by the `tast` executable.
    *   [`common/`](src/chromiumos/tast/common/) - Packages shared between some
        combination of `tast/` and `local/` and `remote/` from the [tast-tests]
        repository.

Tests are located in the [tast-tests] repository.

## Documentation

For more details, see the [docs](docs/) subdirectory.

Package documentation is available at [godoc.org].

[quickstart]: docs/quickstart.md
[overview]: docs/overview.md
[Go in Chromium OS]: http://www.chromium.org/chromium-os/developer-guide/go-in-chromium-os
[tast-tests]: ../tast-tests/
[godoc.org]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast
