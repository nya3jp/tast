# Chrome Precondition (go/tast-precondition)

This document provides a list of existing preconditions.
See the [Chrome Precondition section] for the usage.

[Chrome Precondition]: writing_tests.md#chrome-precondition

## Existing preconditions

The following preconditions are defined:

*   `LoggedIn()` - Chrome is already logged in when a test is run.
*   `LoggedInVideo()` - Chrome is started withs special flags for video tests
    and is already logged in when a test is run.
