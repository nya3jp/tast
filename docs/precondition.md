# Chrome Precondition (go/tast-precondition)

This document provides a list of existing preconditions.
See the [Chrome Precondition] section for their usages.

[Chrome Precondition]: writing_tests.md#Chrome-Precondition

## Existing preconditions

The following preconditions are defined:

*   `LoggedIn()` - Chrome is already logged in when a test is run.
*   `LoggedInVideo()` - Chrome is started withs special flags for video tests
    and is already logged in when a test is run.
