# Getting Code Reviews for Tast Tests (go/tast-reviews)

## Summary

To submit a Tast test change, please follow the following steps.

Firstly, do a **self-review** by going through the [self-review checklist]
below. This will save your time by avoiding typical code review comments in the
later step.

Once the self-review is done, send your change to the following two sets of
reviewers:

1.  **Test owners**. If your change modifies an existing test, send it to one of
    the owners of the test (those listed in the `Contacts` field of the test
    declaration, excluding yourself). If your change introduces a new test, send
    it to your team member(s) who will co-own the test. Getting reviews from the
    test owners makes sure the change is good from the perspective of feature
    experts [(details)](#Why-are-test-owner-reviews-required).

2.  **Tast reviewers**. Send your change to tast-owners@google.com. In a few
    minutes, the code review is assigned to one or more [Tast reviewers] using
    the [gwsq] bot. Getting reviews from the Tast reviewers makes sure the
    change is good from the perspective of Tast test experts
    [(details)](#Why-are-Tast-reviewer-reviews-required).

After getting LGTM from both reviewers, submit the change via the Commit Queue.

[self-review checklist]: #Self_review-checklist
[Tast reviewers]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/refs/heads/master/OWNERS
[gwsq]: https://goto.google.com/gwsq-gerrit


## Self-review checklist

### Stabilize existing tests before adding new tests

As [announced on the tast-users list], we have a policy that teams cannot add
additional [mainline] tests until their existing informational tests have been
stabilized and promoted to [critical] tests. Any new [mainline] test being added
must have a clear plan for being promoted to a [critical] test.

New test authors should start out by stabilizing an existing informational test.
Doing so will expose them to Tast coding conventions, making it easier to write
new code in the future.

[announced on the tast-users list]: https://groups.google.com/a/chromium.org/d/topic/tast-users/dmS2OWp2bYU/discussion
[mainline]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md#manually_added-attributes
[critical]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md#manually_added-attributes

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
[go vet], [golint] and [tast-lint]) as repo upload hooks to find obvious
mistakes and style guide violations before time-consuming manual code reviews.

Except for WIP changes, always make sure to run repo upload hooks. Changes
failing to pass lint checks may not be reviewed.

[gofmt]: https://golang.org/cmd/gofmt/
[goimports]: https://godoc.org/golang.org/x/tools/cmd/goimports
[go vet]: https://golang.org/cmd/vet/
[golint]: https://github.com/golang/lint
[tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/chromiumos/cmd/tast-lint/

### Run unit tests

Unit tests are not run automatically in repo upload hooks for technical reasons.
You need to run them manually by the following command in the Chrome OS chroot:

```
~/trunk/src/platform/tast/fast_build.sh -T
```

CLs breaking unit tests are rejected by the (Pre) Commit Queue.

### Run Pre-CQ

In the Gerrit UI, set the Commit-Queue+1 label to run the Pre Commit Queue
(Pre-CQ) for your change. You do not have to wait for the Pre-CQ to finish
before sending code reviews to reviewers.

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

### Why are test owner reviews required?

Test owner reviews make sure your change is good from the perspective of feature
experts.

Test owners know the tested feature a lot better than Tast reviewers, obviously.

### Why are Tast reviewer reviews required?

Tast reviewer reviews make sure your change is good from the perspective of Tast
test experts.

Tast reviewers are engineers from various teams in Chrome OS who have experience
with writing and reviewing Tast tests. Their review insures that tests are
written using learned best practices for test execution speed, stability and
maintainability.

### Why are all Tast tests owned by Tast team, not by my team?

Tast tests are in fact owned by the team listed in the Contacts field, not by
the Tast team.

It is simply due to technical reasons that the tast-tests repository's OWNERS
file lists Tast reviewers only. Changes to a test should be reviewed by both
the owning person/team listed in the test's Contacts field and Tast reviewers.

### Tests are failing in the Commit Queue. Can I skip Tast reviewer reviews for demoting/disabling them?

Yes. In the case of emergency, please feel free to add
`Exempt-From-Owner-Approval: <reason>` line to the change description to bypass
Tast reviewer reviews.

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
