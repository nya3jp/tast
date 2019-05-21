# Getting Code Reviews for Tast Tests (go/tast-reviews)

## Before requesting a code review (checklist)

Have you...

*   [stabilized existing tests before adding new tests?]
*   [run pre-upload hooks to catch any problems with Go/Tast style?]
*   [added a BUG= line to your change?]
*   [if your change is large, split it into separate, smaller changes?]

[stabilized existing tests before adding new tests?]: #Stabilize-existing-tests-before-adding-new-tests
[run pre-upload hooks to catch any problems with Go/Tast style?]: #Follow-Go_Tast-style
[added a BUG= line to your change?]: #Associate-changes-to-bug-tracker-issues
[if your change is large, split it into separate, smaller changes?]: #Do-not-make-large-changes

### Stabilize existing tests before adding new tests

As [announced on the tast-users list], we have a policy that teams cannot add
additional [mainline] tests until their existing informational tests have been
stabilized and promoted to the CQ. Any new [mainline] test being added must have
a clear plan for being promoted to the CQ.

New test authors are recommended to start out by trying to stabilize an
existing informational test. Doing so will expose you to Tast coding
conventions and make it easier to write new code in the future.

[announced on the tast-users list]: https://groups.google.com/a/chromium.org/d/topic/tast-users/dmS2OWp2bYU/discussion
[mainline]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md#manually_added-attributes

### Follow Go/Tast style

Test code should be formatted by [gofmt] and checked by [go vet], [golint] and
[tast-lint]. These tools are configured to run as pre-upload hooks, so don't
skip them.

Tast code should also follow Go's established best practices as described by
these documents:

*   [Effective Go]
*   [Go Code Review Comments]

[gofmt]: https://golang.org/cmd/gofmt/
[go vet]: https://golang.org/cmd/vet/
[golint]: https://github.com/golang/lint
[tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/chromiumos/cmd/tast-lint/
[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments

### Associate changes to bug tracker issues

All Tast test changes should be associated with bug tracker issues (typically
on the [Chromium bug tracker]). When adding a new test, it is recommended to
file an issue to track promoting the test and using it to track flakiness
issues that need to be resolved.

[Chromium bug tracker]: https://crbug.com/

### Do not make large changes

(See also [go/small-cls])

If you are working on a large test, don't send the entire test out as a single,
gigantic change. Split your changes into smaller changes that can be submitted
separately.

Changes that consist of small, focused changes are always easier and faster to
review than large, complicated changes. The length of a code review increases
much faster than linearly with the size of the change.

If your change is only large because of test data (e.g. baseline information
about expected processes or files), it's fine to keep it all together.

[go/small-cls]: https://goto.google.com/small-cls


## Requesting a code review

Before you send a code review to the Tast team, make sure to get your changes
reviewed by someone on your team. Your team is usually more familiar with your
feature than we are.

After passing team reviews, please send your changes to
`tast-owners@google.com`, the alias of [Tast OWNERS] who can approve your
change.

[Tast OWNERS]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/OWNERS
