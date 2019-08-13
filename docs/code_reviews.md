# Getting Code Reviews for Tast Tests (go/tast-reviews)

## Summary

To submit a Tast test change, please follow these steps:

1.  **Self review**. Make sure your change is ready for reviews by going through
    the [self-review checklist] below.
2.  **Team review**. Send your change to your team members and get it reviewed.
    [(why?)](#Why-are-team-reviews-required)
3.  **Tast review**. After getting LGTM from your team members, send your change
    to tast-owners@google.com. In a few minutes, the gwsq bot reassigns the code
    review to one or more [Tast reviewers]. LGTM from a Tast reviewer is
    required to submit the change. [(why?)](#Why-are-Tast-reviews-required)
4.  Submit via the Commit Queue.

[self-review checklist]: #Self_review-checklist
[Tast reviewers]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/refs/heads/master/OWNERS


## Self-review checklist

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

### Run repo upload hooks

Tast repositories are configured to run many linters ([gofmt], [goimports],
[go vet], [golint] and [tast-lint]) as repo upload hooks. Those linters are
there to save your time by finding obvious mistakes and style guide violations
before time-consuming human code reviews.

Except for WIP changes, always make sure to run repo upload hooks. Changes
failing to pass lint checks won't be reviewed by Tast reviewers.

[gofmt]: https://golang.org/cmd/gofmt/
[goimports]: https://godoc.org/golang.org/x/tools/cmd/goimports
[go vet]: https://golang.org/cmd/vet/
[golint]: https://github.com/golang/lint
[tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/chromiumos/cmd/tast-lint/

### Check frequent code review comments

Tast code should follow Go's established best practices as described by these
documents:

*   [Effective Go]
*   [Go Code Review Comments]

There are quite a few Tast-specific best practices described by the
[Tast: Writing Tests] document. Below is the list of best practices pointed out
most often in code reviews:

*   [Avoid passing around testing.State]
*   [Use testing.Poll instead of testing.Sleep]
*   [Do not skip tests at runtime]
*   [Use preconditions]

[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments
[Tast: Writing Tests]: writing_tests.md
[Avoid passing around testing.State]: writing_tests.md#test-subpackages
[Use testing.Poll instead of testing.Sleep]: writing_tests.md#contexts-and-timeouts
[Do not skip tests at runtime]: writing_tests.md#device-dependencies
[Use preconditions]: writing_tests.md#preconditions


## FAQ

### Why are team reviews required?

Team reviews make sure your change is good from the perspective of feature
experts.

Team reviews also have many benefits not provided by Tast reviews:

*   Your team members know your feature a lot better than Tast reviewers.
*   Your team members are typically co-located with you and their review latency
    would be shorter.
*   Your team members can also get used to Tast by reviewing Tast changes.

It is recommended to get LGTM in team reviews before sending to Tast reviewers
to maximize these benefits.

### Why are Tast reviews required?

Tast reviews make sure your change is good from the perspective of Tast test
experts.

Tast reviewers are engineers from various teams in Chrome OS who have written
and reviewed many Tast tests. We want you to write fast, stable and maintainable
integration tests, but we know that it is not easy to do at all. We are here to
help you do so by sharing best practices we have learned.

### Why are all Tast tests owned by Tast team, not by my team?

Tast tests are in fact owned by the team listed in the Contacts field, not by
the Tast team.

It is simply due to technical reasons that the tast-tests repository's OWNERS
file lists Tast reviewers only. Changes to a test should be reviewed by both
the owning person/team listed in the test's Contacts field and Tast reviewers.

### Tests are failing in the Commit Queue. Can I skip Tast reviews for demoting/disabling them?

Yes. In the case of emergency, please feel free to add
`Exempt-From-Owner-Approval: <reason>` line to the change description to bypass
Tast reviews.

In any case, please remember to file a tracking bug for demotion/disablement and
CC the change/bug to the test contacts listed in the Contacts field. If you need
to chump a change, please get an approval from the sheriffs and leave a comment
in Gerrit for reference.

### How can I become a Tast reviewer?

Please write and review Tast changes to get used to Go, Tast and integration
test best practices in general. Typically you need to write more than 10
non-trivial changes to feel familiar with Tast.

Once you feel ready to go, please send a mail to chromeos-velocity@google.com to
join shadow reviews. See [go/tast-shadow-review] for details of the process.
Upon graduating from shadow reviews, you will be added to [Tast reviewers].

[go/tast-shadow-review]: https://goto.google.com/tast-shadow-review
