# Tast: Writing Tests (go/tast-writing)

[TOC]

## Adding tests

### Test names

Tests are identified by names like `login.Chrome` or `platform.ConnectToDBus`.
The portion before the period, called the _category_, is the final component of
the test's package name, while the portion after the period is the name of the
exported Go function that implements the test.

Test function names should follow [Go's naming conventions], and [acronyms
should be fully capitalized]. Test names should not end with `Test`, both
because it's redundant and because the `_test.go` filename suffix is reserved in
Go for unit tests.

Test names are automatically derived from tests' package and function names and
should not be explicitly specified when defining tests.

[Go's naming conventions]: https://golang.org/doc/effective_go.html#names
[acronyms should be fully capitalized]: https://go.dev/wiki/CodeReviewComments#initialisms

### Code location

Public tests built into the default `cros` local and remote [test bundles] are
checked into the [tast-tests repository] under the
[src/go.chromium.org/tast-tests/cros/local/bundles/cros/] and
[src/go.chromium.org/tast-tests/cros/remote/bundles/cros/] directories (where
src/go.chromium.org/tast-tests/cros/ can also be accessed by the `cros` symlink
at the top of the repository).
Private tests are checked into private repositories such as the
[tast-tests-private repository], and built into non-`cros` test bundles.

Tests are categorized into packages based on the functionality that
they exercise; for example, the [ui package] contains local tests that exercise
the ChromeOS UI. The category package needs to be directly under the bundle
package. Thus the category package path should be matched with
`go.chromium.org/tast/core/(local|remote)/bundles/(?P<bundlename>[^/]+)/(?P<category>[^/]+)`.

A local test named `ui.DoSomething` should be defined in a file named
`src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/do_something.go` (i.e. convert the
test name to lowercase and insert underscores between words).

Support packages used by multiple test categories located in
[src/go.chromium.org/tast-tests/cros/local/] and [src/go.chromium.org/tast-tests/cros/remote/], alongside the
`bundles/` directories. For example, the [chrome package] can be used by local
tests to interact with Chrome.

If there's a support package that's specific to a single category, it's often
best to place it underneath the category's directory. See the [Scoping and
shared code] section.

Tast-tests-private repository `go.chromium.org/tast-tests-private/...` should not import packages bundle in `go.chromium.org/tast-tests/cros/local/bundles/...` and `go.chromium.org/tast-tests/cros/remote/bundles/...`

Packages outside `go.chromium.org/tast-tests/cros/local/...` should not import packages in `go.chromium.org/tast-tests/cros/local/...`, and
packages outside `go.chromium.org/tast-tests/cros/remote/...` should not import packages in `go.chromium.org/tast-tests/cros/remote/...`.
If local and remote packages should share the same code, put them in `go.chromium.org/tast-tests/cros/common/...`.

[test bundles]: overview.md#Test-bundles
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD
[tast-tests-private repository]: https://chrome-internal.googlesource.com/chromeos/platform/tast-tests-private/+/HEAD
[src/go.chromium.org/tast-tests/cros/local/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/
[src/go.chromium.org/tast-tests/cros/remote/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/
[ui package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/
[src/go.chromium.org/tast-tests/cros/local/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/
[src/go.chromium.org/tast-tests/cros/remote/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/
[chrome package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/chrome/
[Scoping and shared code]: #Scoping-and-shared-code

### Test registration

A test needs to be registered by calling `testing.AddTest()` in the test entry
file, which is located directly under a category package. The registration
needs to be done in `init()` function in the file. The registration should be
declarative, which means:
- `testing.AddTest()` should be the only statement of `init()`'s body.
- `testing.AddTest()` should take a pointer of a `testing.Test` composite literal.

Each field of testing.Test should be constant-like. Fields should not be set
using the invocation of custom functions (however, append() is allowed), or
using variables. In particular, we say constant-like is any of these things:

- An array literal of constant-like.
- A go constant.
- A literal value.
- A var defined as an array literal of go constants or literal values (N.B. not
  general constant-likes).
- A var forwarding (set to) another constant-like var.
- A call to append on some constant-likes.
- A call to hwdep.D, but please apply the spirit of constant-like to the
  arguments to hwdep.D.

The test registration code will be similar to the following:

```go
// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DoSomething,
		Desc:         "Does X to verify Y",
		Contacts:     []string{"team@google.com", "me@chromium.org"},
		BugComponent: "b:12345",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
	})
}

func DoSomething(ctx context.Context, s *testing.State) {
	// The actual test goes here.
}
```

Tests have to specify the descriptions in `Desc`, which should be a string literal.

Tests have to specify email addresses of persons and groups who are familiar
with those tests in `Contacts`. The first element of the slice should be a group
alias for the team ultimately responsible for the test. Subsequent elements
should be individuals or groups who can be contacted for code reviews, bugs,
and any issue with the test's usage. To help aid triage and on-call rotations,
partner owned tests must specify a Google email contact that can be
reached by on-call rotations. Any google.com or chromium.org groups listed should
accept email posts from non-members within the organization. Users who no longer
work on Chrome OS or with test's owning team should remove themselves as a
contact.

Tests have to specify a `BugComponent`, which should be a string with a prefix
indicating the bug tracker. The string's contents point to the location where
bugs regarding the test should initially be filed. A prefix is used to
distinguish between different bug trackers. For Buganizer, use "b:" plus
the componentid, e.g. "b:1034625". Ensure that a *componentid* is used, not a
specific bug id.

Tests have to specify [attributes] to describe how they are used in ChromeOS
testing. A test belongs to zero or more groups by declaring attributes with
`group:`-prefix. Typically functional tests belong to the mainline group by
declaring the `group:mainline` attribute. New mainline tests should have the
`informational` attribute, as tests without this attribute will block the Commit
Queue on failure otherwise. The `Attr` fields should be an array literal of
string literals.

The `SoftwareDeps` field lists [software dependencies] that should be satisfied
in order for the test to run. Its value should be an array literal of string
literals or (possibly qualified) identifiers which are constant value.

Tests should always set the `Timeout` field to specify the maximum duration for
which Func may run before the test is aborted. If not specified, a reasonable
default will be used, but tests should not depend on it.

#### Disabling tests

If a test has no `group:*` attribute assigned it will be effectively disabled,
it will not be run by any automation.
If a test needs to be disabled leave a comment in the test source with the
reason. If applicable, create a bug explaining under what circumstances the
test can be enabled.

[Contacts]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Test
[attributes]: test_attributes.md
[software dependencies]: test_dependencies.md

### Adding new test categories

When adding a new test category, you must update the test bundle's `imports.go`
file (either [local/bundles/cros/imports.go] or [remote/bundles/cros/imports.go]) to
underscore-import the new package so its `init` functions will be executed to
register tests.

[local/bundles/cros/imports.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/imports.go
[remote/bundles/cros/imports.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/imports.go

## Coding style and best practices

Test code should be formatted by [gofmt] and checked by [go vet], [staticcheck]
and [tast-lint]. These tools are configured to run as pre-upload hooks, so
don't skip them.

Tast code should also follow Go's established best practices as described by
these documents:

*   [Effective Go]
*   [Go Code Review Comments]

The [Go FAQ] may also be helpful. Additional resources are linked from the [Go
Documentation] page.

[gofmt]: https://golang.org/cmd/gofmt/
[go vet]: https://golang.org/cmd/vet/
[staticcheck]: https://staticcheck.dev
[tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/go.chromium.org/tast/core/cmd/tast-lint/
[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://go.dev/wiki/CodeReviewComments
[Go FAQ]: https://golang.org/doc/faq
[Go Documentation]: https://golang.org/doc/

### Documentation

Packages and exported identifiers (e.g. types, functions, constants, variables)
should be documented by [Godoc]-style comments. Godoc comments are optional for
test functions, since the `Test.Desc` field already contains a brief description
of the test.

[Godoc]: https://blog.golang.org/godoc-documenting-go-code

### Unit tests

Support packages should be exercised by unit tests when possible. Unit tests can
cover edge cases that may not be typically seen when using the package, and they
greatly aid in future refactorings (since it can be hard to determine the full
set of Tast-based tests that must be run to exercise the package). See [How to
Write Go Code: Testing] and [Go's testing package] for more information about
writing unit tests for Go code. The [Best practices for writing ChromeOS unit
tests] document contains additional suggestions that may be helpful (despite
being C++-centric).

Setting `FEATURES=test` when emerging a test bundle package
(`tast-local-tests-cros` or `tast-remote-tests-cros`) will run all unit tests
for the corresponding packages in the `tast-tests` repository (i.e.
`go.chromium.org/tast-tests/cros/local/...` or `go.chromium.org/tast-tests/cros/remote/...`, respectively).

During development, the [fast_build.sh] script can be used to quickly build and
run tests for a single package or all packages.

[How to Write Go Code: Testing]: https://golang.org/doc/code.html#Testing
[Go's testing package]: https://golang.org/pkg/testing/
[Best practices for writing ChromeOS unit tests]: https://www.chromium.org/chromium-os/developer-library/guides/testing/unit-tests/
[fast_build.sh]: modifying_tast.md#fast_build_sh

### Import

Entries in import declaration must be grouped by empty line, and sorted in
following order.

- Standard library packages
- Third-party packages
- chromiumos/ packages

In each group, entries must be sorted in the lexicographical order. For example:

```go
import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast-tests/cros/local/chrome"
)
```

Note that, although github.com and golang.org are different domains, they
should be in a group.

This is how `goimports --local=chromiumos/` sorts. It may be valuable to run
the command. Note that, 1) the command preserves existing group. So, it may
be necessary to remove empty lines in import() in advance, and 2) use the
command to add/remove import entries based on the following code. The path
resolution may require setting `GOPATH` properly.

## Test structure

As seen in the test declaration above, each test is comprised of a single
exported function that receives a [testing.State] struct. This is defined in the
[Tast testing package] (not to be confused with [Go's `testing` package] for
unit testing) and is used to log progress and report failures.

[testing.State]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#State
[Tast testing package]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/go.chromium.org/tast/core/testing/

### Startup and shutdown

If a test requires the system to be in a particular state before it runs, it
should include code that tries to get the system into that state if it isn't
there already. Previous tests may have aborted mid-run; it's not safe to make
assumptions that they undid all temporary changes that they made.

Tests should also avoid performing unnecessary de-initialization steps on
completion: UI tests should leave Chrome logged in at completion instead of
restarting it, for example. Since later tests can't safely make assumptions
about the initial state of the system, they'll need to e.g. restart Chrome again
regardless, which takes even more time. In addition to resulting in a faster
overall running time for the suite, leaving the system in a logged-in state
makes it easier for developers to manually inspect it after running the test
when diagnosing a failure.

Note that tests should still undo atypical configuration that leaves the system
in a non-fully-functional state, though. For example, if a test needs to
temporarily stop a service, it should restart it before exiting.

Use [defer] statements to perform cleanup when your test exits. `defer` is
explained in more detail in the [Defer, Panic, and Recover] blog post.

Put more succintly:

> Assume you're getting a reasonable environment when your test starts, but
> don't make assumptions about Chrome's initial state. Similarly, try to leave
> the system in a reasonable state when you go, but don't worry about what
> Chrome is doing.

[defer]: https://tour.golang.org/flowcontrol/12
[Defer, Panic, and Recover]: https://blog.golang.org/defer-panic-and-recover

### Contexts and timeouts

Tast uses [context.Context] to implement timeouts. A test function takes as its
first argument a [context.Context] with an associated deadline that expires when
the test's timeout is reached. The default timeout is 2 minutes for [local tests]
and 5 minutes for [remote tests]. The context's `Done` function returns a [channel]
that can be used within a [select] statement to wait for expiration, after which
the context's `Err` function returns a non-`nil` error.

The [testing.Poll] function makes it easier to honor timeouts while polling for
a condition:

```go
if err := testing.Poll(ctx, func (ctx context.Context) error {
	var url string
	if err := MustSucceedEval(ctx, "location.href", &url); err != nil {
		return testing.PollBreak(errors.Wrap(err, "failed to evaluate location.href"))
	}
	if url != targetURL {
		return errors.Errorf("current URL is %s", url)
	}
	return nil
}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
	return errors.Wrap(err, "failed to navigate")
}
```

Return a [testing.PollBreak] error to stop the polling. Useful when you get an
unexpected error inside the polling.

Sleeping without polling for a condition is discouraged, since it makes tests
flakier (when the sleep duration isn't long enough) or slower (when the duration
is too long). If you really need to do so, use [testing.Sleep] to honor the context
timeout.

Any function that performs a blocking operation should take a [context.Context]
as its first argument and return an error if the context expires before the
operation finishes.

Several blog posts discuss these patterns in more detail:

*   [Go Concurrency Patterns: Context]
*   [Go Concurrency Patterns: Timing out, moving on]

Note: there is an old equivalent "golang.org/x/net/context" package, but for
consistency, the built-in "context" package is preferred.

> As a rule of thumb, a timeout should be **double of the expected worst case
> performance**. If you're unsure, measure time multiple times in the worst case
> scenario and double that. Do not use timeouts to catch performance
> regressions. Instead consider writing a performance test.
> When a test hits a timeout that was sufficient before, investigate why it hit
> the timeout before increasing it.

The performance and worst case scenario can be obtained using the [time
calculation script]. It parses the test result logs to obtain the average and
max time from various executions.

[context.Context]: https://golang.org/pkg/context/
[channel]: https://tour.golang.org/concurrency/2
[local tests]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast/src/go.chromium.org/tast/core/internal/bundle/local.go
[remote tests]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast/src/go.chromium.org/tast/core/internal/bundle/remote.go
[select]: https://tour.golang.org/concurrency/5
[testing.Poll]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Poll
[testing.PollBreak]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#PollBreak
[testing.Sleep]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Sleep
[Go Concurrency Patterns: Context]: https://blog.golang.org/context
[Go Concurrency Patterns: Timing out, moving on]: https://blog.golang.org/go-concurrency-patterns-timing-out-and
[time calculation script]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/tools/test_time_calculation.py

### Reserve time for clean-up task

For any function with a corresponding clean-up function, prefer using the [defer]
statement to keep the two function calls close together (see the
[Startup and shutdown](#startup-and-shutdown) section for detail).

Create a separate clean up context using [ctxutil.Shorten] to make sure to reserve
enough time for your deferred functions to run even when the test context timed out.
```go
cleanupCtx := ctx
ctx, cancel := ctxutil.Shorten(ctx, timeForCleanup)
defer cancel()
...
service := createService(ctx, ...)
defer func(ctx context.Context) {
  if err := service.Close(ctx); err != nil {
    s.Error("Failed to close service: ", err)
  }
}(cleanupCtx)
```

Make sure you catch and report errors in cleanup functions. You can use
`s.Error` to continue running the current deferred function and `return`
if your cleanup function needs to exit early. Avoid using `s.Fatal` in
deferred functions.

[ctxutil.Shorten]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/ctxutil#Shorten

### Concurrency

Concurrency is rare in integration tests, but it enables doing things like
watching for a D-Bus signal that a process emits soon after being restarted. It
can also sometimes be used to make tests faster, e.g. by restarting multiple
independent Upstart jobs simultaneously.

The preferred way to synchronize concurrent work in Go programs is by passing
data between [goroutines] using a [channel]. This large topic is introduced in
the [Share Memory by Communicating] blog post, and the [Go Concurrency Patterns]
talk is also a good summary. [The Go Memory Model] provides guarantees about the
effects of memory reads and writes across goroutines.

[goroutines]: https://tour.golang.org/concurrency/1
[Share Memory by Communicating]: https://blog.golang.org/share-memory-by-communicating
[Go Concurrency Patterns]: https://talks.golang.org/2012/concurrency.slide
[The Go Memory Model]: https://golang.org/ref/mem

### Scoping and shared code

Global variables in Go are [scoped at the package level] rather than the file
level:

> The scope of an identifier denoting a constant, type, variable, or function
> ... declared at top level (outside any function) is the package block.

As such, all tests within a package like `platform` or `ui` share the same
namespace. It is ok to declare top level unexported symbols
(e.g. functions, constants, etc), but please be careful of conflicts. Also,
please avoid referencing identifiers declared in other files; otherwise
`repo upload` will fail with lint errors.

If you need to share functionality between tests in the same package, please
introduce a new descriptively-named subpackage; see e.g. the [chromecrash]
package within the `ui` package, used by the [ui.ChromeCrashLoggedIn] and
[ui.ChromeCrashNotLoggedIn] tests. Subpackages are described in more detail
later in this document. Importing a subpackage is allowed only in the category
package containing it; otherwise `repo upload` will fail with lint errors.

[scoped at the package level]: https://golang.org/ref/spec#Declarations_and_scope
[chromecrash]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/chromecrash/
[ui.ChromeCrashLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/chrome_crash_logged_in.go
[ui.ChromeCrashNotLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/chrome_crash_not_logged_in.go

### Test consolidation

Much praise has been written for verifying just one thing per test. A quick
sampling of internal links:

*   [TotT 227]
*   [TotT 324]
*   [TotT 339]
*   [TotT 520]
*   [Unit Testing Best Practices Do's and Don'ts]

While this is sound advice for fast-running, deterministic unit tests, it isn't
necessarily always the best approach for integration tests:

*   There are unavoidable sources of non-determinism in ChromeOS integration
    tests. DUTs can experience hardware or networking issues, and flakiness
    becomes more likely as more tests are run.
*   When a lengthy setup process is repeated by many tests in a single suite,
    lab resources are consumed for a longer period of time and other testing is
    delayed.

If you need to verify multiple related aspects of a single feature that requires
a time-consuming setup process like logging in to Chrome, starting Android, or
launching a container, it's often preferable to write a single test that just
does the setup once and then verifies all aspects of the feature. As described
in the [Errors and Logging] section, multiple errors can be reported by a single
test, so coverage need not be reduced when tests are consolidated and an early
expectation fails.

For lightweight testing that doesn't need to interact with Chrome or restart
services, it's fine to use fine-grained tests — there's almost no per-test
overhead in Tast; the overhead comes from repeating the same slow operations
_within_ multiple tests.

*** aside
If all the time-consuming setup in your test suite is covered by a tast
[fixtures], then splitting your test into multiple fine-grained tests
will incur negligible overhead.
***

[TotT 227]: http://go/tott/227
[TotT 324]: http://go/tott/324
[TotT 339]: http://go/tott/339
[TotT 520]: http://go/tott/520
[Unit Testing Best Practices Do's and Don'ts]: http://go/unit-test-practices#behavior-testing-dos-and-donts
[Errors and Logging]: #errors-and-logging
[fixtures]: #Fixtures

### Device dependencies

A Tast test either passes (by reporting zero errors) or fails (by reporting one
or more errors, timing out, or panicking). If a test requires functionality that
isn't provided by the DUT, the test is skipped entirely.

Avoid writing tests that probe the DUT's capabilities at runtime, e.g.

```go
// WRONG: Avoid testing for software or hardware features at runtime.
func CheckCamera(ctx context.Context, s *testing.State) {
    if !supports720PCamera() {
        s.Log("Skipping test; device unsupported")
        return
    }
    // ...
}
```

This approach results in the test incorrectly passing even though it actually
didn't verify anything. (Tast doesn't let tests report an "N/A" state at runtime
since it would be slower than skipping the test altogether and since it will
prevent making intelligent scheduling decisions in the future about where tests
should be executed.)

Instead, specify [software dependencies] when declaring tests:

```go
// OK: Specify dependencies when declaring the test.
func init() {
    testing.AddTest(&testing.Test{
        Func: CheckCamera,
        SoftwareDeps: []string{"camera_720p", "chrome"},
        // ...
    })
}
```

The above document describes how to define new dependencies.

Also, there is an API Features which allows tests to get information regarding
DUT features. However, it is purely used for informational purpose only. Do not
use it to alter test behavior. Use it for only for informational purpose. Use
[parameterized tests] for tests to have different behavior for different
DUT features.

If a test depends on the DUT being in a specific configurable state (e.g. tablet
mode), it should put it into that state. For example, [chrome.ExtraArgs] can be
passed to [chrome.New] to pass additional command-line flags (e.g.
`--force-tablet-mode=touch_view`) when starting Chrome.

The [tast-users mailing list] is a good place to ask questions about test
dependencies.

[chrome.ExtraArgs]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#ExtraArgs
[chrome.New]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#New
[tast-users mailing list]: https://groups.google.com/a/chromium.org/forum/#!forum/tast-users
[parameterized tests]: #Parameterized-tests

### Fixtures

Sometimes a lengthy setup process (e.g. restarting Chrome and logging in, which
takes at least 6-7 seconds) is needed by multiple tests. Rather than running the
same setup for each of those tests, tests can declare the shared setup, which is
named "fixtures" in Tast.

Tests sharing the same fixture run consecutively. A fixture implements several
_lifecycle methods_ that are called by the framework as it executes tests
associated with the fixture. `SetUp()` of the fixture runs once just before the
first of them starts, and `TearDown()` is called once just after the last of
them completes unless `s.Fatal` or `s.Error` are called during `SetUp()`.
`Reset()` runs after each but the last test to roll back changes
a test made to the environment.

* Fixture `SetUp()`
* Test 1 runs
* Fixture `Reset()`
* Test 2 runs
* Fixture `Reset()`
* ...
* Fixture `Reset()`
* Test N runs
* Fixture `TearDown()`

`Reset()` should be a light-weight and idempotent operation. If it fails
(returns a non-nil error), framework falls back to `TearDown()` and `SetUp()` to
completely restart the fixture.
Tests should not leave too much change on system environment, so that the next
`Reset()` does not fail.

*** aside
Currently Reset errors do not mark a test as failed. We plan to change this
behavior in the future ([b/187795248](http://b/187795248)).
***

Fixtures also have `PreTest()` and `PostTest()` methods, which run before and
after each test. They get called with [`testing.FixtTestState`] with which you
can report errors as a test. It's a good place to set up logging for individual
test for example.

For details of these fixture lifecycle methods, please see the GoDoc
[`testing.FixtureImpl`].

Each test can declare its fixture by setting [`testing.Test.Fixture`] an fixture
name. The fixture's `SetUp()` returns an arbitrary value that can be obtained by
calling `s.FixtValue()` in the test. Because `s.FixtValue()` returns an
`interface{}`, type assertion is needed to cast it to the actual type.
However, `s.FixtValue()` will always return nil when local tests/fixtures
try to access values from remote fixtures because Tast does not know the actual
types of fixture values to deserialize them. Therefore, there is another
function `s.FixtFillValue(v, any)` which requires user to pass in a pointer,
and it will store the deserialized result in the value pointed to by pointer.

Fixtures are composable. A fixture can declare its parent fixture with
`testing.Fixture.Parent`. Parent's `SetUp()` is executed before the fixture's
`SetUp()` is executed, parent's `TearDown()` is executed after the fixtures's
`TearDown()`, and so on. Fixtures can use the parent's value in the same way
tests use it.

Local tests/fixtures can depend on a remote fixture if they live in test bundles
with the same name (e.g. local `cros` and remote `cros`).

Fixtures are registered by calling [`testing.AddFixture`] with [`testing.Fixture`]
struct in `init()`. `testing.Fixture.Name` specifies the fixture name,
`testing.Fixture.Impl` specifies implementation of the fixture,
`testing.Fixture.Parent` specifies the parent fixture if any,
`testing.Fixture.SetUpTimeout` and the like specify methods' timeout,
and the other fields are analogous to `testing.Test`.

Fixtures can be registered outside bundles directory. It's best to initialize
and register fixtures outside bundles if it is shared by tests in multiple
categories.

#### Examples

* Rather than calling [chrome.New] at the beginning of each test, tests can
declare that they require a logged-in Chrome instance by setting
[`testing.Test.Fixture`] to "[chromeLoggedIn]" in `init()`. This enables Tast to
just perform login once and then share the same Chrome instance with all tests
that specify the fixture. See the [chromeLoggedIn] documentation for more
details, and [example.ChromeFixture] for a test using the fixture.

* If you want a new Chrome fixture with custom options, call
[`testing.AddFixture`] from [chrome/fixture.go] with different options, and give
it a unique name.

#### Theory behind fixtures

On designing composable fixtures, understanding the theory behind fixtures might
help.

Let us think of a space representing all possible system states. A fixture's
purpose is to change the current system state to be in a certain subspace. For
example, the fixture [chromeLoggedIn]'s purpose is to provide a clean
environment similar to soon after logging into a Chrome session. This can be
rephased that there's a subspace where "the system state is clean similar to
soon after logging into a Chrome session" and the fixture's designed to change
the system state to some point inside the subspace.

To denote these concepts a bit formally: let `U` be a space representing all
possible system states. Let `f` be a function that maps a fixture to its target
system state subspace. Then, for any fixture `X`, `f(X) ⊆ U`. Note that `f(F)`
is a subspace of `U`, not a point in `U`; there can be some degrees of freedom
in a resulting system state.

A fixture's property is as follows: if a test depends on a fixture `F` directly
or indirectly, it can assume that the system state is in `f(F)` on its start.
This also applies to fixtures: if a fixture depends on a fixture `F` directly or
indirectly, it can assume that the system state is in `f(F)` on its setup.

To fulfill this property, all fixtures should satisfy the following rule: if a
fixture `X` has a child fixture `Y`, then `f(X) ⊇ f(Y)`. Otherwise, calling
`Y`'s reset may put the system state to one not accepted by `X`, failing to
fulfill the aforementioned property.

#### Parameterized Fixtures

Similar to tests, parameterized fixtures are also supported. They can be used to
define multiple similar fixtures with different features/options.

To parameterize a fixture, specify a slice of `testing.FixtureParam` in
the `Params` field on fixture registration. If `Params` is non-empty,
`testing.AddFixture` will expand the fixture into one or more tests
corresponding to each item in `Params` by merging `testing.Fixture` and
`testing.FixtureParam`.

Here is an [example with normal parameterized fixtures]
and an [example of a test using the normal parameterized fixtures].

`FixtureParam` should be a literal in general since fixture registration
should be [declarative]. However, a fixture may need to support a different
combination of features or options. For fixtures that support a lot of
different features/options, fixture writers and users may have a hard time to
figure out what parameters have been declared for a particular combination
of features/options. To ease the effort for composing a fixture with a lot
of features/options, factory-like functions are allowed to be used for
`FixtureParam` declaration and reference to the parameterized fixture.
Even with the exception, the fixture writers and test writers should always
make sure the function always returns the same fixture name for the same build.
Otherwise, the generated metadata will produce the wrong data and cause
issues in the test executions. Also, the factory function should be simple and
the name of the parameter should allow users to identify the combination of
features/options easily.

Here is an [example of parameterized fixtures with a factory],
and an [example of a test use a parameterized fixture with a factory].


#### Preconditions

Preconditions, predecessor of fixtures, are not recommended for new tests.

[`testing.Fixture`]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/internal/testing#Fixture
[`testing.FixtureImpl`]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/internal/testing#FixtureImpl
[`testing.FixtTestState`]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/internal/testing#FixtTestState
[chrome.New]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome#New
[chromeLoggedIn]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/chrome/fixture.go
[`testing.Test.Fixture`]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/internal/testing#Test.Fixture
[chrome/fixture.go]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/chrome/fixture.go
[example.ChromeFixture]: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/chrome_fixture.go
[`testing.AddFixture`]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#AddFixture
[example with normal parameterized fixtures]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/meta/fixture.go#61
[example of a test using the normal parameterized fixtures]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/remote_fixt_param.go
[example of parameterized fixtures with a factory]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/meta/fixture.go#72
[example of a test use a parameterized fixture with a factory]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/local_fixt_param.go


## Hooks/Remote Root Fixture
Tast [`remote root fixture`] which allows test writers to add hooks that
affect every test (except test with Pre-Condition) in a Tast session.
Those hooks can perform different functions such as checking DUT state
between tests.

All tests and fixtures will depend on the [`remote root fixture`] directly
or indirectly. The Setup phase of the [`remote root fixture`] which will
be run at the beginning of the execution of a Tast bundle. It can allow
different hooks to be run during different phases of the remote root
fixture: Setup, PreTest,  Reset, PostTest, and TearDown. Hook writers
can do something applicable to all tests.

Tast's fixture tree is constructed separately while running
tests for different bundles. Since a Tast session may include tests
from different bundles, the setup for the root fixture may be invoked
multiple times during a Tast session.  Also, if the retries flag of
Tast is set to be greater than zero, Tast will need to re-construct
and run the fixture tree. Hence, the setup of the root fixture will
be invoked again.

The root fixture will allow us to add framework level setups and
monitors. For example, we can start servod on the labstation at the
beginning of the running a bundle so that all tests in the bundle
can use servo.  It will also allow test writers to create hooks that
affect every test in a Tast session. It will allow users to create a
customized environment for each test suite.

### Adding a Hook
All hooks should be in the [`hooks directory`]. Hooks can depend on
remote and common Tast libraries. On the other hand, tests or fixtures
should not have any dependencies on hooks.


#### addHook
The function addHook for users to add a hook to the root fixture. It is
intentionally not to export this function’s symbol so that all hooks and
the [`remote root fixture`] will be created in the same package. This will
allow Tast team to have better control what hooks are allowed.

#### HookImpl
HookImpl is an interface that users have to implement so that the
root fixture can invoke the corresponding action in each step of a
fixture process.

```go
type HookImpl interface {
        // SetUp will be called during root fixture Setup.
        SetUp(ctx context.Context, s *HookState) error
        // Reset will be called during root fixture Reset.
        Reset(ctx context.Context) error
        // PreTest will be called during root fixture PreTest.
        PreTest(ctx context.Context, s *HookTestState) error
        // PostTest will be called during root fixture PostTest.
        PostTest(ctx context.Context, s *HookTestState) error
        // TearDown will be called during root fixture TearDown.
        TearDown(ctx context.Context, s *HookState) error
}
```

#### orderedHooks
To avoid non-deterministic order of the execution, a variable
“orderHooks” will be used to maintain the order of execution. To add
a new hook, the author needs to add the name of the hook to the list.
Otherwise, the hook will not be executed. The order of the execution
On the other hand, PostTest and TearDown will be executed in the
reverse order of this list.

```go
var orderedHooks []string
```

#### HookState
HookState includes certain fixture state information that is available
for each hook to be used during Setup and Teardown.

#### HookTestState
HookTestState includes fixture state information that is available for
each hook to be used during PreTest and PostTest.

#### Service Dependencies
Since the [`remote root fixture`] is a remote fixture, all hooks are
running on the host side. Some hooks may need to use GPRC services that
are running on the DUT to perform actions on the DUT. Users can add
ServiceDeps to the root fixture if their hooks need GPRC services.

#### Code Review
Since all hooks affect the entire Tast session, they affect all tests.
Therefore, any additions and changes of hooks will require approval
from a member of the Tast team. Also, at least one expert reviewer will
be needed for each hook related CL.

### Example of Adding A Hook
Here is a [`hook example`].


#### Add Hook
All hooks should be added in the “init” function of a file.

```go
func init() {
    addHook(&Hook{
        Name:         "exampleHook",
        Desc:         "Demonstrate how to use hook",
        Contacts:     []string{"tast-core@google.com"},
        BugComponent: "b:1034522",
        Impl:         &testhook{},
    })
}
```

#### Create A Runtime Variable
In the [`hook example`], the hook will use a Tast global variable to
determine whether actions should be performed. This variable is for
this example only.

```go
var shouldRun = testing.RegisterVarString(
    "hooks.example.shouldrun",
    "",
    "A variable to decide whether example hook should run",
)
```

#### Implement The Hook
The implementation of a hook needs to include five functions:
Setup, PreTest, PostTest, Reset, and Teardown. In this example,
all five functions  will only perform action if the variable
hooks.example.shouldrun is set to true.

```go
type testhook struct {
    shouldRun bool
}

// SetUp will be called during root fixture Setup.
func (h *testhook) SetUp(ctx context.Context, s *HookState) error {
    h.shouldRun = shouldRun.Value() == "true"
    if !h.shouldRun {
        return nil
    }
    testing.ContextLog(ctx, "testhook Setup")
    return nil
}

// Reset will be called during root fixture Reset.
func (h *testhook) Reset(ctx context.Context) error {
    if !h.shouldRun {
        return nil
    }
    testing.ContextLog(ctx, "testhook Reset")
    return nil
}

// PreTest will be called during root fixture PreTest.
func (h *testhook) PreTest(ctx context.Context, s *HookTestState) error {
    if !h.shouldRun {
            return nil
    }
    testing.ContextLog(ctx, "testhook PreTest: ", s.TestName())
    return nil
}

// PostTest will be called during root fixture PostTest.
func (h *testhook) PostTest(ctx context.Context, s *HookTestState) error {
    if !h.shouldRun {
        return nil
    }
    testing.ContextLog(ctx, "testhook PostTest: ", s.TestName())
    return nil
}

// TearDown will be called during root fixture TearDown.
func (h *testhook) TearDown(ctx context.Context, s *HookState) error {
    if !h.shouldRun {
        return nil
    }
    testing.ContextLog(ctx, "testhook TearDown")
    return nil
}
```

#### Add the Example Hook To the Execution List
New hook will not be executed until it is added to the list
“orderedHooks” of the [`remote root fixture`].


```go
var orderedHooks []string = []string{
    "exampleHook",
}
```

#### Turning On Example Hook During Test
This example show how to turn on the example hook with Tast command line.

```
tast run -var=hooks.example.shouldrun=true <dut> <tests>
```

### Policy
Since hooks can affect every test and fixture in a Tast session, we need
to be careful about what hooks can go in the remote root fixture. Here
is the policy that we have at this moment. The policy may be revised
in future.

* All hook related CLs have to be reviewed and approved by Tast team members.

* An 1-pager should be sent to Tast team to review in advance.

* For more complicated hooks, proper design documents should be reviewed.

* Most hooks should be guarded by Tast global runtime variables which means
  that most hooks should only be triggered when the corresponding runtime
  variables are set.


[`remote root fixture`]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/hooks/root_fixture.go
[`hook example`]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/hooks/example.go
[`hooks directory`]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/remote/hooks


## Common testing patterns

### Table-driven tests

It is sometimes the case that multiple scenarios with very slight differences
should be tested. In this case you can write a [table-driven test], which is a
common pattern in Go unit tests. [testing.State.Run] can be used to start a
subtest.

```go
for _, tc := range []struct {
    format   string
    filename string
    duration time.Duration
}{
    {
        format:   "VP8",
        filename: "sample.vp8",
        duration: 3 * time.Second,
    },
    {
        format:   "VP9",
        filename: "sample.vp9",
        duration: 3 * time.Second,
    },
    {
        format:   "H.264",
        filename: "sample.h264",
        duration: 5 * time.Second,
    },
} {
    s.Run(ctx, tc.format, func(ctx context.Context, s *testing.State) {
        if err := testPlayback(ctx, tc.filename, tc.duration); err != nil {
            s.Error("Playback test failed: ", err)
        }
    })
}
```

[table-driven test]: https://go.dev/wiki/TableDrivenTests
[testing.State.Run]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#State.Run

## Errors and logging

The [testing.State] struct provides functions that tests may use to report their
status:

*   `Log` and `Logf` record informational messages about the test's progress.
*   `Error` and `Errorf` record errors and mark the test as failed but allow it
    to continue, similar to [Google Test]'s `EXPECT_` set of macros. Multiple
    errors may be reported by a single test.
*   `Fatal` and `Fatalf` record errors and stop the test immediately, similar to
    the `ASSERT_` set of macros.

Note that higher-level functions for stating expectations and assertions are not
provided; this was a conscious decision. See ["Where is my favorite helper
function for testing?"] from the [Go FAQ]. That answer refers to [Go's testing
package] rather than Tast's, but the same reasoning and suggestions are
applicable to Tast tests.

[Google Test]: https://github.com/google/googletest
["Where is my favorite helper function for testing?"]: https://golang.org/doc/faq#testing_framework

### When to log

When you're about to do something that could take a while or even hang, log a
message using `Log` or `Logf` first. This both lets developers know what's
happening when they run your test interactively and helps when looking at logs
to investigate timeout failures.

On the other hand, avoid logging unnecessary information that would clutter the
logs. If you want to log a verbose piece of information to help determine the
cause of an error, only do it after the error has occurred. Also, if you are
interested in which part of a test is time-consuming, please see the
[Reporting timing] section for details.

See the [fmt package]'s documentation for available "verbs".

[fmt package]: https://golang.org/pkg/fmt/
[Reporting timing]: #Reporting-timing

<a name="log-vs-logf"></a>

### Log/Error/Fatal vs. Logf/Errorf/Fatalf

`Log`, `Error`, and `Fatal` should be used in conjunction with a single string
literal or when passing a string literal followed by a single value:

```go
s.Log("Doing something slow")
s.Log("Loading ", url)
s.Error("Encountered an error: ", err)
s.Fatal("Everything is broken: ", err)
```

`Logf`, `Errorf`, and `Fatalf` should only be used in conjunction with `printf`-style
format strings:

```go
s.Logf("Read %q from %v", data, path)
s.Errorf("Failed to load %v: %v", url, err)
s.Fatalf("Got invalid JSON object %+v", obj)
```

When concatenating a string and a value using default formatting, use
`s.Log("Some value: ", val)` rather than the more-verbose
`s.Logf("Some value: %v", val)`.

The same considerations apply to `testing.ContextLog` vs. `testing.ContextLogf`.

<a name="error-pkg"></a>

### Error construction

To construct new errors or wrap other errors, use the [go.chromium.org/tast/core/errors]
package rather than standard libraries (`errors.New`, `fmt.Errorf`) or any other
third-party libraries. It records stack traces and chained errors, and leaves
nicely formatted logs when tests fail.

To construct a new error, use [errors.New] or [errors.Errorf].

```go
errors.New("process not found")
errors.Errorf("process %d not found", pid)
```

To construct an error by adding context to an existing error, use [errors.Wrap] or [errors.Wrapf].

```go
errors.Wrap(err, "failed to connect to Chrome browser process")
errors.Wrapf(err, "failed to connect to Chrome renderer process %d", pid)
```

To examine sentinel errors which may be `Wrap`ed, use [errors.Is] or
[errors.As]. The usage is the same as the functions with the same names in the
official errors package.

Sometimes you may want to define custom error types, for example, to inspect and
react to errors. In that case, embed `*errors.E` to your custom error struct.

```go
type CustomError struct {
    *errors.E
}

if err := doSomething(); err != nil {
    return &CustomError{E: errors.Wrap(err, "something failed")}
}
```

It is recommended to wrap when you cross package boundary, which represents
some kind of barrier beneath which everything is an implementation detail.
Otherwise it is fine to return an error without wrapping, if you can't really
add much context to make debugging easier. Use your best judgement to decide
wrap or not.

Following quotes from
*[The Go programming language] 5.4.1 Error-Handling Strategies*
are useful to design good errors:

> - When designing error messages, be deliberate, so that each one is a meaningful description of the problem with sufficient and relevant detail.
> - In general, the call `f(x)` is responsible for reporting the attempted operation `f` and the argument value `x` as they relate to the context of the error.
> - The caller is responsible for adding further information that it has but the call `f(x)` does not.

[go.chromium.org/tast/core/errors]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors
[errors.New]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#New
[errors.Errorf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#Errorf
[errors.Wrap]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#Wrap
[errors.Wrapf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#Wrapf
[errors.Is]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#Is
[errors.As]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors#As
[The Go programming language]: https://www.gopl.io/

### Formatting

<a name="error-fmt"></a>

Please follow [Go's error string conventions] when producing `error` values.

> Error strings should not be capitalized (unless beginning with proper nouns or
> acronyms) or end with punctuation, since they are usually printed following
> other context.

For example:

```go
if err := doSomething(id); err != nil {
	return errors.Wrapf(err, "doing something to %q failed", id)
}
```

<a name="log-fmt"></a>

Log and error messages printed by tests via `testing.State`'s `Log`, `Logf`,
`Error`, `Errorf`, `Fatal`, or `Fatalf` methods, or via `testing.ContextLog` or
`testing.ContextLogf`, should be capitalized phrases without any trailing
punctuation that clearly describe what is about to be done or what happened:

```go
s.Log("Asking Chrome to log in")
...
if err != nil {
	s.Fatal("Failed to log in: ", err)
}
s.Logf("Logged in as user %v with ID %v", user, id)
```

<a name="common-fmt"></a>

In all cases, please avoid multiline strings since they make logs difficult to
read. To preserve multiline output from an external program, please write it to
an [output file] instead of logging it.

When including a path, URL, or other easily-printable value in a log message or
an error, omit leading colons or surrounding quotes:

```go
s.Logf("Trying to log in up to %d time(s)", numLogins)
errors.Errorf("%v not found", path)
```

Use quotes when including arbitrary data that may contain hard-to-print
characters like spaces:

```go
s.Logf("Successfully read %q from %v", data, path)
```

Use a colon followed by a space when appending a separate clause that contains
additional detail (typically an error):

```go
s.Error("Failed to log in: ", err)
```

Semicolons are appropriate for joining independent clauses:

```go
s.Log("Attempt failed; trying again")
```

[Go's error string conventions]: https://go.dev/wiki/CodeReviewComments#error-strings
[output file]: #Output-files

### Support packages

Support packages should not record test failures directly. Instead, return
`error` values (using the [errors package]) and allow tests to decide
how to handle them. Support packages' exported functions should typically take
[context.Context] arguments and use them to return an error early when the
test's deadline is reached and to log informative messages using
`testing.ContextLog` and `testing.ContextLogf`.

Similarly, support packages should avoid calling `panic` when errors are
encountered. When a test is running, `panic` has the same effect as `State`'s
`Fatal` and `Fatalf` methods: the test is aborted immediately. Returning an
`error` gives tests the ability to choose how to respond.

The [Error handling and Go] and [Errors are values] blog posts offer guidance on
using the `error` type.

[errors package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/errors
[Error handling and Go]: https://blog.golang.org/error-handling-and-go
[Errors are values]: https://blog.golang.org/errors-are-values

### Test subpackages

The above guidelines do not necessarily apply to test subpackages that are
located in subdirectories below test files. If a subpackage actually contains
the test implementation (typically because it's shared across several tests),
it's okay to pass `testing.State` to it so it can report test errors itself.

Subpackages are typically aware of how they will be used, so an argument can be
made for letting them abort testing using `Fatal` or even `panic` in cases where
it improves code readability (e.g. for truly exceptional cases like I/O
failures). Use your best judgement.

Note that it's still best to practice [information hiding] and pass only as much
data is needed. Avoid passing `testing.State` when it's not actually necessary:

*   If a function just needs the output directory, pass a path.
*   If a function just needs to log its progress, pass a `context.Context` so it
    can call `testing.ContextLog`.

[information hiding]: https://en.wikipedia.org/wiki/Information_hiding

### Reporting timing

The [timing package] can be used to measure and report the time taken by
different "stages" of a test. It helps you identify which stage takes an
unexpectedly long time to complete.

An example to time a test with two stages:

```go
func TestFoo(ctx context.Context, s *testing.State) {
    // Tast framework already adds a stage for the test function.
    stageA(ctx)
    stageB(ctx)
}

func stageA(ctx context.Context) {
    ctx, st := timing.Start(ctx, "stage_a")
    defer st.End()
    ...
}

func stageB(ctx context.Context) {
    ctx, st := timing.Start(ctx, "stage_b")
    defer st.End()
    ...
}
```

By default, the result will be written to `timing.json` (see [timing#Log.Write]
for details) in the Tast [results dir]. The above example will generate:

```json
[4.000, "example.TestFoo", [
    [1.000, "stage_a"],
    [3.000, "stage_b"]]]
```

[timing package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/timing
[timing#Log.Write]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/timing#Log.Write
[results dir]: running_tests.md#Interpreting-test-results

### Logged in users home directory

Within the scope of a test, it might be useful to put files in the user's
`Downloads` or `MyFiles` directory. To achieve this, there are 2 helper methods
that calculate the logged in user's hash and return the path to either
`Downloads` or `MyFiles`. To use these, do the following:

```go
import "go.chromium.org/tast-tests/cros/local/cryptohome"

downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
if err != nil {
  s.Fatal("Failed to get users Download path: ", err)
}
```

There is an alternative `MyFilesPath` if you require the `MyFiles` location
directly. Please avoid using the `/home/chronos/user` path directly as these are
being deprecated.

## Output files

Tests can write output files that are automatically copied to the host system
that was used to initiate testing:

```go
func WriteOutput(s *testing.State) {
	if err := os.WriteFile(filepath.Join(s.OutDir(), "my_output.txt"),
		[]byte("Here's my output!"), 0644); err != nil {
		s.Error(err)
	}
}
```

As described in the [Running tests] document, a test's output files are copied
to a `tests/<test-name>/` subdirectory within the results directory.

[Running tests]: running_tests.md

### Performance measurements

The [perf] package is provided to record the results of performance tests.  See
the [perf] documentation for more details.

[perf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/perf

## Data files

Tests can register ancillary data files that will be copied to the DUT and made
available while the test is running; consider a JavaScript file that Chrome
loads or a short binary audio file that is played in a loop, for example.

### Internal data files

Small non-binary data files should be directly checked into a `data/`
subdirectory under the test package as _internal data files_. Prefix their names
by the test file's name (e.g. `data/user_login_some_data.txt` for a test file
named `user_login.go`) to make ownership obvious.

Per the [Chromium guidelines for third-party code], place
(appropriately-licensed) data that wasn't created by Chromium developers within
a `third_party` subdirectory under the `data` directory.

[Chromium guidelines for third-party code]: https://chromium.googlesource.com/chromium/src.git/+/HEAD/docs/adding_to_third_party.md

### External data files

Larger data files like audio, video, or graphics files should be stored in
Google Cloud Storage and registered as _external data files_ to avoid
permanently bloating the test repository. External data files are not installed
to test images but are downloaded at run time by `local_test_runner` on DUT.

To add external data files, put _external link files_ named
`<original-name>.external` in `data/` subdirectory whose content is JSON in the
[external link format].

For example, a data file belonging to a test named `ui.UserLogin` in the default
`cros` bundle might be declared in `user_login_some_image.jpg.external` with the
following content:

```
{
  "url": "gs://chromiumos-test-assets-public/tast/cros/ui/user_login_some_image_20181210.jpg",
  "size": 12345,
  "sha256sum": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

> Old versions of external data files should be retained indefinitely in Google
> Cloud Storage so as to not break tests on older system images. Include the
> date as a suffix in the filename to make it easy to add a new version when
> needed, e.g. `user_login_data_20180812.bin`.

The commands `du -b $FILE` and `sha256sum $FILE` can calculate the size and
sha256sum respectively.

If data files are produced as [build artifacts of ChromeOS], they can be also
used as external data files. However, build artifacts are available only for
ChromeOS images built by official builders; for developer builds, tests
requiring build artifacts will fail.

An example external link file to reference a build artifact is below:

```
{
  "type": "artifact",
  "name": "license_credits.html"
}
```

To upload a file to Google Cloud Storage you can use the [`gsutil cp`] command.

Example gsutil command to upload the user login image above:

```
$ gsutil cp /tmp/your/local_file/user_login_some_image_20181210.jpg gs://chromiumos-test-assets-public/tast/cros/ui/user_login_some_image_20181210.jpg
```

**IMPORTANT:** Don't use space in the file name until this bug is fixed [b/271155369].

[b/271155369]: https://buganizer.corp.google.com/issues/271155369

To list all uploaded versions of the file, use the `gsutil ls -a` command.

External files are cached in two locations: /usr/local/share/tast/data_pushed on
the DUT and /tmp/tast/devserver on the host machine. To ensure the reproducibility
of tests and prevent stale cache data from being served, cloud storage files should
never be overwritten once they have been used in a CQ run or dry-run. If
overwriting a cloud storage file, remember to manually clear the cache folders
before running Tast tests to prevent stale files from being served.

[external link format]: https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/go.chromium.org/tast/core/internal/extdata/extdata.go
[example.DataFiles]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/data_files.go
[build artifacts of ChromeOS]: https://goto.google.com/cros-build-google-storage
[`gsutil cp`]: https://cloud.google.com/storage/docs/gsutil/commands/cp

### Internal vs. external

As internal data files are much easier to view and modify than external data
files, it's usually better to check in textual data. Only store binaries as
external data.

### Executables

If your test depends on outside executables, use Portage to build and package
those executables separately and include them in test ChromeOS system images.
Tast [intentionally](design_principles.md) does not support compiling or
deploying other packages that tests depend on.

### Sharing data files between test packages

If a data file is needed by a support package that's used by tests in multiple
packages, it should be stored in a `data` subdirectory within the support
package and symlinked into each test package's `data` subdirectory. See the
[media_session_test.html] file used by the [mediasession package] and shared by
the [ui.PlayPauseChrome] and [arc.MediaSessionGain] tests, for example.

[media_session_test.html]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/chrome/mediasession/data/media_session_test.html
[mediasession package]: https://pkg.go.dev/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/local/chrome/mediasession
[ui.PlayPauseChrome]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/ui/play_pause_chrome.go
[arc.MediaSessionGain]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/main/src/go.chromium.org/tast-tests/cros/local/bundles/cros/arc/media_session_gain.go

### Using data files in tests

To register data files (regardless of whether they're checked into the test
repository or stored externally), in your test's `testing.AddTest` call, set the
`testing.Test` struct's `Data` field to contain a slice of data file names
(omitting the `data/` subdirectory, and the `.external` suffix for external data
files):

```go
testing.AddTest(&testing.Test{
	...
	Data: []string{"user_login_data.bin"},
	...
})
```

Later, within the test function, pass the same filename to [testing.State]'s
`DataPath` function to receive the path to the data file on the DUT:

```go
b, err := os.ReadFile(s.DataPath("user_login_data.bin"))
```

See the [example.DataFiles] test for a complete example of using both local and
external data files.

[example.DataFiles]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/data_files.go

## Runtime variables

Occasionally tests need to access dynamic or secret data (i.e. *out-of-band*
data), and that's when runtime variables become useful.

### Setting values

To set runtime variables, add (possibly repeated) `-var=name=value` flags to
`tast run`.

### Accessing values

Tast users can access runtime variables in two different ways. One way
is to declare global runtime variables which can be used by all testing
entities: services, fixtures, library functions and tests. Other entities
can use the variables by importing the package that defines the variables.
The other way is to declare test runtime variables which can be used by
fixture and tests.

#### Global runtime variables (recommended for new code)
To declare a global runtime variable, use testing.RegisterVarString in an
entity. It should be a top-level variable declaration which should include
the name of the variable, default value and description. A duplicate of the
variable name in the same bundle will result in an error during registration
when a bundle starts.  Other files can access the variable by importing the
package that contains the declaration of the variable.

Example:

```go
package example

...

var exampleStrVar = testing.RegisterVarString(
        "example.AccessVars.globalString",
        "Default value",
        "An example variable of string type",
)

...

func AccessVars(ctx context.Context, s *testing.State) {
        strVal := exampleStrVar.Value()
}
```

All variables should have the prefix “<package_name>.” to avoid name collision.
If one violates this convention, runtime error will happen.

#### Test runtime variables
To declare test runtime variables, set the `testing.Test` struct's `Vars`
or `VarDeps` field inside your tests' `testing.AddTest` call.
`Vars` specifies optional runtime variables, and `VarDeps` specifies required
runtime variables to run the test. `VarDeps` should be the default choice,
and `Vars` should be used only when there's a fallback in case the variables
are missing.

`Vars` and `VarDeps` should be an array literal of string literals or constants.
The test can later access the values by calling `s.Var` or `s.RequiredVar`
methods.

For variables only used in a single test, prefix them with the test name
(e.g. `arc.Boot.foo` for a variable used only in `arc.Boot`).
For variables used from multiple tests, prefix them with the category name which mainly uses the variable
(e.g. `arc.foo`). Such variables can be used from any tests, not only ones in the same category.

Variables without a dot in its name are called global variables. They are set by the framework, and individual tests don't have control over them.
Other variables should follow these rules:

* Variable name should have the form of `foo.Bar.something` or `foo.something`, where `something` matches `[A-Za-z][A-Za-z0-9_]*`
* Only the test `foo.Bar` can access `foo.Bar.something`
* Any tests can access `foo.something`

If one violates this convention, runtime error will happen.

### Skipping tests if a variable is not set.

If you wish to skip tests if a variable is not set, your should use
`VarDeps` field inside those tests' `testing.AddTest` call regardless which
methods you choose to access the variable.

When runtime variables in `VarDeps` are missing, by default the test fails
before it runs. `-maybemissingvars=<regex>` can be used to specify possibly
missing runtime variables and if every missing runtime variable in `VarDeps`
matches with the regex, the test is skipped.

### Secret variables

This feature is for internal developers, who has access to `tast-tests-private` package.

#### What is it

This feature allows you to store secret key/value pairs in a private repository, and use them from public tests.

For example, tests no longer have to be private just because they access secret GAIA credentials.

#### How to do it

Let `foo.Bar` be the test which should access secret username and password.

If the variables are only used from the test, create the file `tast-tests-private/vars/foo.Bar.yaml` with the contents:

```Yaml
foo.Bar.user: someone@something.com
foo.Bar.password: whatever
```

If the values are shared among tests, create `foo.yaml` file instead.

```Yaml
foo.user: someone@something.com
foo.password: whatever
```

Then the test can access the variables just like normal variables assigned to the `tast` command with `-var`.
Secret variables cannot be used to define global variables.

**Don't log secrets in tests** to avoid possible data leakage.

```go
func init() {
	testing.AddTest(&testing.Test{
		Func:     Bar,
	...
		VarDeps: []string{"foo.Bar.user", "foo.Bar.password"},
		// or foo.user, foo.password
	})
}

func Bar(ctx context.Context, s *testing.State) {
	user := s.RequiredVar("foo.Bar.user")
	...
}
```

See [example.SecretVars](https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/example/secret_vars.go) for working example.

#### Naming convention

* The file defining `foo.Bar.something` should be `foo.Bar.yaml`
* The file defining `foo.something` should be `foo.yaml`

If one violates this convention, Tast linter will complain. Please honor the linter errors.

## Parameterized tests

When multiple scenarios with very slight differences should be tested, the most
common pattern is to write [table-driven tests]. However testing everything in
a single test is sometimes undesirable for several reasons:

*   Tests should have different attributes. For example, we might want
    to set some of them [critical] to avoid regressions, while keeping others
    [informational] due to test flakiness.
*   Tests should declare different dependencies. For example, VP8 playback
    test should declare the "hardware-accelerated VP8 decoding" hardware
    dependency, while other playback tests should declare their respective
    dependencies.
*   Test results should be reported separately. For example, video playback
    performance tests may want to report performance metrics separately for
    different video formats (VP8/VP9/H.264/...).

In such cases, *parameterized tests* can be used to define multiple similar
tests with different test properties.

To parameterize a test, specify a slice of [`testing.Param`] in the `Params`
field on test registration. `Params` should be a literal since test
registration should be [declarative]. If `Params` is non-empty,
`testing.AddTest` expands the test into one or more tests corresponding to
each item in `Params` by merging `testing.Test` and [`testing.Param`] with the
rules described below.

Here is an example of a parameterized test registration:

```go
func init() {
    testing.AddTest(&testing.Test{
        Func:     Playback,
        Desc:     "Tests media playback",
        Contacts: []string{"someone@chromium.org"},
        Attr:     []string{"group:mainline"},
        Params: []testing.Param{{
            Name:      "vp8",
            Val:       "sample.vp8",
            ExtraData: []string{"sample.vp8"},
            ExtraAttr: []string{"informational"},
        }, {
            Name:      "vp9",
            Val:       "sample.vp9",
            ExtraData: []string{"sample.vp9"},
            // No ExtraAttr; this test is critical.
        }, {
            Name:              "h264",
            Val:               "sample.h264",
            ExtraSoftwareDeps: []string{"chrome_internal"}, // H.264 codec is unavailable on ChromiumOS
            ExtraData:         []string{"sample.h264"},
            ExtraAttr:         []string{"informational"},
        }},
    })
}

func Playback(ctx context.Context, s *testing.State) {
    filename := s.Param().(string)
    if err := playback(ctx, filename); err != nil {
        s.Fatal("Failed to playback: ", err)
    }
}
```

`Name` in [`testing.Param`] is appended to the base test name with a leading dot
to compute the test name, just like `category.TestName.parameter_name`.
If `Name` is empty, the base test name is used as-is. `Name` should be in
`lower_snake_case` style. `Name` must be unique within a parameterized test.

`Val` in [`testing.Param`] is an arbitrary value that can be accessed in the
test body via the `testing.State.Param` method. Since it returns the value as
`interface{}`, it should be type-asserted to the original type immediately.
All `Val` in a parameterized test must have the same type.

`Pre` and `Timeout` in [`testing.Param`] are equivalent to those in
`testing.Test`. They can be set only if the corresponding fields in the base
test are not set.

`Extra*` in [`testing.Param`] (such as `ExtraAttr`) contains items added to
their corresponding base test properties (such as `Attr`) to obtain the test
properties.

Because test registration should be declarative as written in
[test registration], `Params` should be an array literal containing `Param`
struct literals. In each `Param` struct, `Name` should be a string literal with
`snake_case` name if present. `ExtraAttr`, `ExtraData`, `ExtraSoftwareDeps` and
`Pre` should follow the rule of the corresponding `Attr`, `Data` ,`SoftwareDeps`
and `Pre` in [test registration].

See documentation of [`testing.Param`] for the full list of customizable
properties.

[table-driven tests]: #Table_driven-tests
[critical]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md
[informational]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md
[`testing.Param`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Param
[declarative]: #Test-registration
[test registration]: #Test-registration

## Remote procedure calls with gRPC

In many cases, remote tests have to run some Go functions on the DUT, possibly
calling some support libraries for local tests (e.g. the [chrome] package).
For this purpose, Tast supports defining, implementing, and calling into [gRPC]
services.

For the general usage of gRPC-Go, see also the [official tutorial].

[chrome]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/chrome/
[gRPC]: https://grpc.io
[official tutorial]: https://grpc.io/docs/tutorials/basic/go/

### Defining gRPC services

gRPC services are defined as protocol buffer files stored under
`tast-tests/src/go.chromium.org/tast-tests/cros/services`. The directory is organized in the
similar way as test bundles at
`tast-tests/src/go.chromium.org/tast-tests/cros/{local,remote}/bundles`. Below is an example of
an imaginary gRPC service `arc.BootService`:

```
tast-tests/src/go.chromium.org/tast-tests/cros/services/
  cros/                   ... test bundle name where this service is included
    arc/                  ... service category name
      gen.go              ... Go file containing go generate directives
      boot_service.proto  ... gRPC service definition
      boot_service.pb.go  ... generated gRPC bindings
```

gRPC services are defined in `.proto` files. `boot_service.proto` would look like:

```proto
syntax = "proto3";

package tast.cros.arc;

import "google/protobuf/empty.proto";

option go_package = "go.chromium.org/tast-tests/cros/services/cros/arc";

// BootService allows remote tests to boot ARC on the DUT.
service BootService {
  // CheckBoot logs into a new Chrome session, starts ARC and waits for its
  // successful boot.
  rpc CheckBoot (CheckBootRequest) returns (google.protobuf.Empty) {}
}

message CheckBootRequest {
  enum AndroidImpl {
    DEFAULT = 0;
    CONTAINER = 1;
    VM = 2;
  }
  // impl specifies which ARC implementation to use.
  AndroidImpl impl = 1;
}
```

Protocol buffers files should follow the
[official protocol buffers style guide], as well as several Tast-specific
guidelines:

*   **File names**: Name `.proto` files in the same way as [test `.go` files].
    For example, a service named `TPMStressService` should be defined in
    `tpm_stress_service.proto`.
*   **Package names**: Protocol buffer package name specified in the `package`
    directive should be `tast.<bundle-name>.<category-name>`. Go package name
    specified in the `option go_package` directive should be
    `go.chromium.org/tast-tests/cros/services/<bundle-name>/<category-name>`.
*   **Service names**: Name services with `Service` suffix.
*   **Message names**: Method request/response messages should be named
    `FooBarRequest`/`FooBarResponse`.
*   **Comments**: Write comments in the [godoc style] since these protocol
    buffers are used only by Tast tests in Go.

`gen.go` is a small file containing a [`go generate` directive] to regenerate
`.pb.go` files, looking like the following:

```go
// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. boot_service.proto

package arc

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/arc
```

To regenerate `.pb.go` files, run the command mentioned in the file in ChromeOS
chroot (remember to replace the last argument of the command with the path to
the directory containing the protocol buffer files). This has to be done
manually whenever `.proto` files are edited. Updated `.pb.go` files should be
included and submitted in CLs adding/modifying/deleting `.proto` files.

[official protocol buffers style guide]: https://developers.google.com/protocol-buffers/docs/style
[test `.go` files]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#code-location
[test functions]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#code-location
[godoc style]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#documentation
[`go generate` directive]: https://golang.org/pkg/cmd/go/internal/generate/

### Implementing gRPC services

gRPC service implementations should be placed at the same location as local
tests, i.e. `tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles`. For example, an
imaginary `arc.BootService` would be implemented in
`tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/arc/boot_service.go`.

gRPC services can be registered with [`testing.AddService`] by passing
[`testing.Service`] containing service descriptions. The most important field is
`Register`, specifying a function to register a gRPC service to [`grpc.Server`].
Below is an implementation of the `arc.BootService`:

```go
// tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/arc/boot_service.go
package arc

func init() {
    testing.AddService(&testing.Service{
        Register: func(srv *grpc.Server, s *testing.ServiceState) {
            pb.RegisterBootServiceServer(srv, &BootService{s})
        },
    })
}

// BootService implements tast.cros.arc.BootService.
type BootService struct {
    s *testing.ServiceState
}

func (*BootService) CheckBoot(ctx context.Context, req *pb.CheckBootRequest) (*empty.Empty, error) {
    ...
}
```

For consistency, please follow these guidelines on implementing gRPC services:

*   **File names**: Name gRPC service implementation `.go` files in the exactly
    same way as [test `.go` files]. For example, a service named
    `TPMStressService` should be implemented in `tpm_stress_service.go`.
    This means that gRPC implementation files always have `_service.go` suffix.
*   **Implementation type**: A type implementing gRPC service should have
    exactly the same name as the service name. The type should be the only
    exported symbol in the `_service.go` file. Exactly one gRPC service
    implementation should be registered in a single file.
*   **Inter-file references**: Similarly to test files, `_service.go` file
    should not refer symbols in different files in the same directory.
    Consequently, a gRPC service has to be implemented in a single file.
    If the file gets too long, please consider introducing a subpackage just
    like tests.

`context.Context` passed to a gRPC method can be used to call some of
`testing.Context*` functions:

*   `testing.ContextLog`, `testing.ContextLogf`, `testing.ContextLogger` work
    fine. Emitted logs are recorded as if they were emitted by a remote test
    that called into a gRPC method.
*   `testing.ContextOutDir` returns a path to a temporary directory. Files saved
    in the directory during a gRPC method call are copied back to the host
    machine's test output directory, as if they were saved by a remote test that
    called into a gRPC method. Note that this function does not allow gRPC
    methods to read output files from a remote test nor previous gRPC method
    calls. Files are overwritten in the case of name conflicts.
*   `testing.ContextSoftwareDeps` does not work. This function is planned to be
    deprecated ([crbug.com/1135996]).

`Register` function receives [`testing.ServiceState`] which you can keep in
a field of the struct type implementing the gRPC service. It allows the service
to access service-specific information, such as runtime variables and data files
(not implemented yet: [crbug.com/1027381]).

[`testing.AddService`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#AddService
[`testing.Service`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#Service
[`grpc.Server`]: https://godoc.org/google.golang.org/grpc#Server
[test `.go` files]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#code-location
[`testing.ServiceState`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/testing#ServiceState
[crbug.com/1135996]: https://crbug.com/1135996
[crbug.com/1027381]: https://crbug.com/1027381

### Calling gRPC services

A remote test should declare in its metadata which gRPC services it will call
into. Undeclared gRPC method calls shall be rejected internally. For example,
an imaginary remote test `arc.RemoteBoot` would be declared as:

```go
func init() {
    testing.AddTest(&testing.Test{
        Func:         RemoteTest,
        SoftwareDeps: []string{"chrome", "android_p"},
        ServiceDeps:  []string{"tast.cros.arc.BootService"},
    })
}
```

Call [`rpc.Dial`] in remote tests to establish a connection to the gRPC server.
On success, it returns a struct containing [`grpc.ClientConn`] with which
you can construct gRPC stubs.

```go
cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
if err != nil {
    s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
}
defer cl.Close(cleanupCtx)

bc := pb.NewBootServiceClient(cl.Conn)

req := pb.CheckBootRequest{Impl: pb.CheckBootRequest_VM}
var res empty.Empty
if err := bc.CheckBoot(ctx, &req, &res); err != nil {
    ...
}
```

[`rpc.Dial`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/go.chromium.org/tast/core/rpc#Dial
[`grpc.ClientConn`]: https://godoc.org/google.golang.org/grpc#ClientConn

### Panics in gRPC services

If a gRPC call fails with "error reading from server: EOF", it may indicate the
service panicked. The panic message is not currently logged or propagated to
the caller. This issue is tracked in [b/187794185].

[b/187794185]: https://buganizer.corp.google.com/issues/187794185

### Notes on designing gRPC services

As with anything involving protos, when updating them, please make sure you
maintain compatibility with older versions of the proto, as it is possible that
you can have a new gRPC server with an old gRPC client, and vice versa. To do
this, just ensure that you follow the principles outlined in
[this](https://protobuf.dev/programming-guides/proto3/#updating) document.

Tast's gRPC services don't necessarily have to provide general-purpose APIs.
It is perfectly fine to define gRPC services specific to a particular test case.
For example, one may want to write a local test which exercises some features,
and a remote test that performs the same testing *after rebooting the DUT*.
In this case, they can put the whole local test content to a subpackage, and
introduce a local test and a gRPC service both of which call into the
subpackage.

[crbug.com/1027368]: https://crbug.com/1027368

## Companion DUTs (Multi-DUTs) Support

Most tests are written to test against one DUT, but multiple DUTs are needed
for tests that are testing the interaction between two or more DUTs. Tast uses
companion DUTs feature to support those tests. Please notice that Tast
currently only support companion DUTs for remote tests, and they are not
accessible by local tests.

### Tast Companion DUT CLI

Users can use the run flag -companiondut to specify a companion DUT to be used
in tests. The flag is repeatable so users can specify more than one companion
DUT. The value of the flag specifies a role and a dut address in this format,
<Role>:<Host>.

#### Examples

Here is an example of how a companion DUT is specified on the Tast command
line.  In this example, primary DUT has the address 127.0.0.1:2222 and the
companion DUT has the address 127.0.0.1:2223.

```
% tast run --companiondut=cd1:127.0.0.1:2223 127.0.0.1:2222 <tests>
```

### Accessing Companion DUTs In A Remote Test

Users can use the function testing.State.CompanionDUT to get the pointer to
dut.DUT of a companion DUT. The function take the role name of the companion
DUT as the input parameter.

#### Examples

```
// CompanionDUTs ensures DUT and companion DUTs are accessible in test.
// Tast command line:
// tast run -build=true -companiondut=cd1:dut1 dut0 meta.CompanionDUTs
func CompanionDUTs(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second*30)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint(), "cros")
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	companionDUT := s.CompanionDUT("cd1")
	if companionDUT == nil {
		s.Fatal("Failed to get companion DUT cd1")
	}
	companionCl, err := rpc.Dial(ctx, companionDUT, s.RPCHint(), "cros")
if err != nil {
		s.Fatal("Failed to connect to the RPC service on the companion DUT: ", err)
	}
	defer companionCl.Close(cleanupCtx)
}
```

### Software/Hardware Dependencies Of Companion DUTs

Users can use HardwareDepsForAll and SoftwareDepsAll of testing.Test to
specify the hardware/software dependencies on all DUTs used in a test.
Furthermore, users can use HardwareDepsForAll and SoftwareDepsAll of
testing.Test to specify the ExtraHardwareDepsForAll and
ExtraSoftwareDepsForAll of testing.Param to specify the hardware/software
dependencies on all DUTs of a parameterized test.

### Example of Specifying Companion Dependencies In A Test

```
type Test struct {
             …
        // SoftwareDepsForAll lists software features of all DUTs that
        // are required to run the test.
        // It is a map of companion roles and software features.
        // The role for primary DUT should be "".
        // The primary DUT software dependency will be the union of
        // SoftwareDeps and SoftwareDepsForAll[""].
        // If any dependencies are not satisfied, the test will be skipped.
        SoftwareDepsForAll map[string][]string

        // HardwareDepsForAll describes hardware features and setup of all
        // DUTs that are required to run the test.
        // It is a map of companion roles and hardware features.
        // The role for primary DUT should be "".
        // The primary DUT hardware dependency will be the union of
        // HardwareDeps and HardwareDepsForAll[""].
        // If any dependencies are not satisfied, the test will be skipped.
        HardwareDepsForAll map[string]hwdep.Deps
        …
}
```

#### Specifying Hardware/Software Dependencies Of Companion DUTs In Parameterized Test

```
type Param struct {
        …

        // ExtraSoftwareDepsForAll lists software features of all DUTs
        // that are required to run the test case for this param,
        // in addition to SoftwareDepsForAll in the enclosing Test.
        // The primary DUT software dependency will be the union of
        // SoftwareDeps, SoftwareDepsForAll[""], ExtraSoftwareDeps and
        // ExtraSoftwareDepsForAll[""].
        // It is a map of companion roles and software features.
        ExtraSoftwareDepsForAll map[string][]string

        // ExtraHardwareDepsForAll describes hardware features and setup
        // companion DUTs that are required to run the test case for this param,
        // in addition to HardwareDepsForAll in the enclosing Test.
        // It is a map of companion roles and hardware features.
        // The role for primary DUT should be ""
        // The primary DUT hardware dependency will be the union of
        // HardwareDeps, HardwareDepsForAll[""], ExtraHardwareDeps and
        // ExtraHardwareDep and ExtraHardwareDepsForAll[""].
        ExtraHardwareDepsForAll map[string]hwdep.Deps
}
```

### Example of Specifying Companion Dependencies In A Test

```
func init() {
        testing.AddTest(&testing.Test{
                Func:         CompanionDepsUsage,
                …
                SoftwareDeps: []string{"chrome"},
                SoftwareDepsForAll: map[string][]string{
                    // Additional primary DUT dependency.
                    // As a result, primary will have dependency on "lacros"
                    "": []string{"lacros"},
                    // Companion DUT 1 dependency.
                    "cd1": []string{"chrome"},
                },
                HardwareDepsForAll: map[string]hwdep.Deps {
                    // Companion DUT 1 dependency.
                    "cd1": hwdep.D(hwdep.InternalDisplay()),
                },
        })
}
```

### Example of Specifying Companion Dependencies In Parameterized Tests

```
func init() {
    testing.AddTest(&testing.Test{
        Func:         CompanionDepsParamUsage,
        …
          SoftwareDeps: []string{"chrome"},
          …
          Params: []testing.Param{
            {
             …
              Name: "P1",
              ExtraSoftwareDepsAll: map[string][]string{
                "cd1": []string{ "android_p"},
              },
             },
              ExtraHardwareDepsForAll: map[string]hwdep.Deps {
                "cd1": hwdep.D(hwdep.InternalDisplay()),
            },
            {
              …
              Name: "p2",
              ExtraSoftwareDepsForAll: map[string][]string{
                "cd1": []string{ "android_vm"},
              },
             },
              ExtraHardwareDepsForAll: map[string]hwdep.Deps {
                "cd1": hwdep.D(hwdep.InternalDisplay()),
              },
            },
         },
    })
}
```





## Utilities

Tast contains many utilities for common operations. Some of them are briefly
described below; see the package links for additional documentation and
examples.

### lsbrelease

The [`lsbrelease`] package provides access to the fields in `/etc/lsb-release`.
Usually Tast tests are not supposed to that information to change their
behavior, so `lsbrelease` contains a list of packages that are allowed to use
it. Attempting to use `lsbrelease` in a package that is not in the allow list
will cause a panic.

[`lsbrelease`]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/src/go.chromium.org/tast/core/lsbrelease/lsbrelease.go

### testexec

The [`testexec`] package provides a convenient interface to run processes on
the DUT. It should be used instead of the standard `os/exec` package.

[`testexec`]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/common/testexec/testexec.go

## Use of third party libraries

1. Add an ebuild to package the code as a Portage package in
[third_party/chromiumos-overlay], and they will effectively review third party
licensing ([example](
https://chromium-review.googlesource.com/c/chromiumos/overlays/chromiumos-overlay/+/1146070)).
2. Add the dependency to tast-build-deps ([example](
https://chromium-review.googlesource.com/c/chromiumos/overlays/chromiumos-overlay/+/1737396)).
Please send this CL to tast-owners@.

Tast doesn't have any official process to review third party libraries.
Just take usual precautions on introducing libraries, such as

- Is the library popular?
- Is the author reliable? Do they respond to issues, solve bugs and accept pull requests?
- Is the API well-documented?
- Does the library cover all your requirements?

If you are in doubt, please feel free to send your proposal to
tast-reviewers@google.com.

When testing locally, remember to `cros_workon --host start tast-build-deps`
and `./update_chroot` to build Tast with your new dependencies.

### Dependencies with 9999 ebuilds

Dependencies are picked up from the host, not any board. So if you have updated
a dependency with a 9999 ebuild, then you'll need to workon start it. For
example if you've added something to system_api:
```
(cr) ~/chromiumos/src/scripts $ cros_workon --host start chromeos-base/system_api tast-build-deps
(cr) ~/chromiumos/src/scripts $ ./update_chroot
```

And cleanup after you're done with local changes:
```
(cr) ~/chromiumos/src/scripts $ cros_workon --host stop chromeos-base/system_api tast-build-deps
```

[third_party/chromiumos-overlay]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/refs/heads/main/dev-go/

### Test promotion process for mainline tests

The group:mainline non-informational tests are run in ChromeOS lab for CQs.

Please see the Google internal link go/tast-add-test (Googler only) for the
promotion process from informational to non-informational.

[go/tast-add-test]: http://goto.google.com/tast-add-test

## Useful Fixtures

Here are some fixtures that may come in handy when writing tast tests.

### Virtual multidisplay testing fixture

There is a multidisplay [testing fixture] designed to be used as a parent
fixture for multidisplay test cases.

This fixture needs to be loaded as a top level parent in order to do it's work
of reloading and configuring the VKMS driver to support multiple virtual
displays.

The pattern used for the ARC and chromium tests can be found in their respective
[fixture definitions].

There is a HasParentState interface that can be implemented by your child
fixture state returned by the SetUp function which can be used and casted within
tests to the VirtualMultidisplayController interface returned by this fixture.

A simple example of this can be seen in the [hotplug test] or a more complex
example in the [arc tests].

[testing fixture]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/virtualmultidisplay/fixture.go
[fixture definitions]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/chrome/fixture.go
[hotplug test]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/virtualmultidisplay/hotplug.go
[arc tests]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/arc/multi_display_virtual.go?autodive=0%2F%2F

#### Requirements

In order for virtual multidisplay tests to run and pass, the VKMS driver with
the configFS patches must be loaded.

This is true for betty boards on 6.1+ already.
