# Preconditions (go/tast-precondition)

This document provides a list of existing preconditions.
See the [Precondition] section for their usages.

[Precondition]: writing_tests.md#Precondition

## Existing preconditions

The following preconditions are defined:

*   [LoggedIn()] - Chrome is already logged in when a test is run.
*   [LoggedInVideo()] - Chrome is started with special flags for video tests and
    already logged in when a test is run.

[LoggedIn()]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#LoggedIn
[LoggedInVideo()]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#LoggedInVideo
