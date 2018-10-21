# Tast: Writing Tests (go/tast-writing)

[TOC]

## Adding tests

### Test names

Tests are identified by names like `ui.ChromeLogin` or `platform.ConnectToDBus`.
The portion before the period, called the _category_, is the final component of
the test's package name, while the portion after the period is the name of the
exported Go function that implements the test. Test function names should follow
[Go's naming conventions], and [acronyms should be fully capitalized]. Test
names are automatically derived and should not be specified when defining tests.

[Go's naming conventions]: https://golang.org/doc/effective_go.html#names
[acronyms should be fully capitalized]: https://github.com/golang/go/wiki/CodeReviewComments#initialisms

### Code location

Public tests built into the default `cros` local and remote [test bundles] are
checked into the [tast-tests repository] under the
[src/chromiumos/tast/local/bundles/cros/] and
[src/chromiumos/tast/remote/bundles/cros/] directories (which may also be
accessed by the `local_tests` and `remote_tests` symlinks at the top of the
repository). Tests are categorized into packages based on the functionality that
they exercise; for example, the [ui package] contains local tests that exercise
the Chrome OS UI.

Support packages used by multiple test categories located in
[src/chromiumos/tast/local/] and [src/chromiumos/tast/remote/], alongside the
`bundles/` directories. For example, the [chrome package] can be used by local
tests to interact with Chrome.

A local test named `ui.MyTest` should be defined in a file named
`src/chromiumos/tast/local/bundles/cros/ui/my_test.go` (i.e. convert the test
name to lowercase and insert underscores between words) with contents similar to
the following:

```go
// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"

	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MyTest,
		Desc: "Does X to verify Y",
		Attr: []string{"informational"},
		Deps: []string{"chrome_login"},
	})
}

func MyTest(ctx context.Context, s *testing.State) {
	// The actual test goes here.
}
```

Tests may specify [attributes] and [software dependencies] when they are
declared. Setting the `informational` attribute on new tests is recommended, as
tests without this attribute will block the Commit Queue on failure otherwise.

If there's a support package that's specific to a single category, it's often
best to place it underneath the category's directory. See the "Scoping and
shared code" section.

[test bundles]: overview.md#Test-bundles
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[src/chromiumos/tast/local/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/
[src/chromiumos/tast/remote/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/bundles/cros/
[ui package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/ui/
[src/chromiumos/tast/local/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/
[src/chromiumos/tast/remote/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/
[chrome package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/chrome/
[attributes]: test_attributes.md
[software dependencies]: test_dependencies.md

### Adding new test categories

When adding a new test category, you must update the test bundle's `main.go`
file (either [local/bundles/cros/main.go] or [remote/bundles/cros/main.go]) to
underscore-import the new package so its `init` functions will be executed to
register tests.

[local/bundles/cros/main.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/main.go
[remote/bundles/cros/main.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/bundles/cros/main.go

## Coding style and best practices

Test code should be formatted by [gofmt] and checked by [go vet]. It should
follow Go's established best practices as described by these documents:

*   [Effective Go]
*   [Go Code Review Comments]

[gofmt]: https://golang.org/cmd/gofmt/
[go vet]: https://golang.org/cmd/vet/
[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments

### Unit tests

Support packages should be exercised by unit tests when possible. Unit tests can
cover edge cases that may not be typically seen when using the package, and they
greatly aid in future refactorings (since it can be hard to determine the full
set of Tast-based tests that must be run to exercise the package). See [Go's
testing package] for more information about writing unit tests for Go code. The
[Best practices for writing Chrome OS unit tests] document contains additional
suggestions that may be helpful (despite being C++-centric).

[Go's testing package]: https://golang.org/pkg/testing/
[Best practices for writing Chrome OS unit tests]: https://chromium.googlesource.com/chromiumos/docs/+/master/unit_tests.md

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

	"github.com/godbus/dbus"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
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

[testing.State]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State
[Tast testing package]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/testing/

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

Tast uses [context.Context] to implement timeouts. The [testing.State] struct's
`Context` function returns a [context.Context] with an associated deadline that
expires when the test's timeout is reached. The context's `Done` function
returns a [channel] that can be used within a [select] statement to wait for
expiration, after which the context's `Err` function returns a non-`nil` error.
The [testing.Poll] function makes it easier to honor timeouts while polling for
a condition.

Any function that performs a blocking operation should take a [context.Context]
(typically as its first argument) and return an error if the context expires
before the operation finishes.

[context.Context]: https://golang.org/pkg/context/
[channel]: https://tour.golang.org/concurrency/2
[select]: https://tour.golang.org/concurrency/5
[testing.Poll]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Poll

### Concurrency

Concurrency is rare in integration tests, but it enables doing things like
watching for a D-Bus signal that a process emits soon after being restarted. It
can also sometimes be used to make tests faster, e.g. by restarting multiple
independent Upstart jobs simultaneously.

The preferred way to synchronize concurrent work in Go programs is by passing
data between [goroutines] using a [channel]. This large topic is introduced in
the [Share Memory by Communicating] blog post. [The Go Memory Model] provides
guarantees about the effects of memory reads and writes across goroutines.

[goroutines]: https://tour.golang.org/concurrency/1
[Share Memory by Communicating]: https://blog.golang.org/share-memory-by-communicating
[The Go Memory Model]: https://golang.org/ref/mem

### Scoping and shared code

Global variables in Go are [scoped at the package level] rather than the file
level:

> The scope of an identifier denoting a constant, type, variable, or function
> ... declared at top level (outside any function) is the package block.

As such, all tests within a package like `platform` or `ui` share the same
namespace. Please avoid adding additional identifiers to that namespace beyond
the core test function that you pass to `testing.AddTest`. Place constants
and helper functions within your test function:

```go
func MyTest(s *testing.State) {
	const (
		someValue = "foo"
		otherValue = "bar"
	)
	doubler := func(x int) int { return 2 * x }
	...
}
```

If you need to share functionality between tests in the same package, please
introduce a new descriptively-named subpackage; see e.g. the [chromecrash]
package within the `ui` package, used by the [ui.ChromeCrashLoggedIn] and
[ui.ChromeCrashNotLoggedIn] tests.

This policy is acknowledged as being non-ideal, and [issue 882022] tracks coming
up with something better.

[scoped at the package level]: https://golang.org/ref/spec#Declarations_and_scope
[chromecrash]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chromecrash/
[ui.ChromeCrashLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chrome_crash_logged_in.go
[ui.ChromeCrashNotLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chrome_crash_not_logged_in.go
[issue 882022]: https://crbug.com/882022

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

*   There are unavoidable sources of non-determinism in Chrome OS integration
    tests. DUTs can experience hardware or networking issues, and flakiness
    becomes more likely as more tests are run.
*   When a lengthy setup process is repeated by many tests in a single suite,
    lab resources are consumed for a longer period of time and other testing is
    delayed.

If you need to verify multiple related aspects of a single feature that requires
a time-consuming setup process like logging in to Chrome, starting Android, or
launching a container, it's often preferable to write a single test that just
does the setup once and then verifies all aspects of the feature. As described
in the next section, multiple errors can be reported by a single test, so
coverage need not be reduced when tests are consolidated and an early
expectation fails.

For lightweight testing that doesn't need to interact with Chrome or restart
services, it's fine to use fine-grained tests — there's almost no per-test
overhead in Tast; the overhead comes from repeating the same slow operations
_within_ multiple tests.

[TotT 227]: http://go/tott/227
[TotT 324]: http://go/tott/324
[TotT 339]: http://go/tott/339
[TotT 520]: http://go/tott/520
[Unit Testing Best Practices Do's and Don'ts]: http://go/unit-test-practices#behavior-testing-dos-and-donts

## Errors and logging

The [testing.State] struct provides functions that tests may use to report their
status:

*   `Log` and `Logf` record informational messages about the test's progress.
*   `Error` and `Errorf` record errors and mark the test as failed but allow it
    to continue, similar to [Google Test]'s `EXPECT_` set of macros. Multiple
    errors may be reported by a single test.
*   `Fatal` and `Fatalf` record errors and stop the test immediately, similar to
    the `ASSERT_` set of macros.

[Google Test]: https://github.com/google/googletest

### When to log

When you're about to do something that could take a while or even hang, log a
message using `Log` or `Logf` first. This both lets developers know what's
happening when they run your test interactively and helps when looking at logs
to investigate timeout failures.

On the other hand, avoid logging unnecessary information that would clutter the
logs. If you want to log a verbose piece of information to help determine the
cause of an error, only do it after the error has occurred.

See the [fmt package]'s documentation for available "verbs".

[fmt package]: https://golang.org/pkg/fmt/

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

### Error construction

To construct new errors or wrap other errors, use the [chromiumos/tast/errors]
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

[chromiumos/tast/errors]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors
[errors.New]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#New
[errors.Errorf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#Errorf
[errors.Wrap]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#Wrap
[errors.Wrapf]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#Wrapf

### Formatting

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

On the other hand, log and error messages printed by tests via `testing.State`,
`testing.ContextLog`, and `testing.ContextLogf` should be capitalized phrases
without any trailing punctuation:

```go
s.Log("Asking Chrome to log in")
...
if err != nil {
	s.Fatal("Failed to log in: ", err)
}
s.Logf("Logged in as user %q with ID %v", user, id)
```

In both cases, avoid multiline strings since they make logs difficult to read.

[Go's error string conventions]: https://github.com/golang/go/wiki/CodeReviewComments#error-strings

### Support packages

Support packages should not record test failures directly. Instead, return
`error` values (using the [errors package]) and allow tests to decide
how to handle them. Support packages' exported functions should typically take
[context.Context] arguments and use them to return an error early when the
test's deadline is reached and to log informative messages using
`testing.ContextLog` and `testing.ContextLogf`.

The [Error handling and Go] and [Errors are values] blog posts offer guidance on
using the `error` type.

[errors package]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors
[Error handling and Go]: https://blog.golang.org/error-handling-and-go
[Errors are values]: https://blog.golang.org/errors-are-values

## Output files

Tests can write output files that are automatically copied to the host system
that was used to initiate testing:

```go
func WriteOutput(s *testing.State) {
	if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "my_output.txt"),
		[]byte("Here's my output!"), 0644); err != nil {
		s.Error(err)
	}
}
```

As described in the [Running tests] document, a test's output files are copied
to a `tests/<test-name>/` subdirectory within the results directory.

[Running tests]: running_tests.md

## Data files

Tests can register ancillary data files that will be copied to the DUT and made
available while the test is running; consider a JavaScript file that Chrome
loads or a short binary audio file that is played in a loop, for example.

### Internal data files

Small non-binary data files should be checked into a `data/` subdirectory under
the test package as _internal data files_. Prefix their names by the test file's
name (e.g. `data/my_test_some_data.txt` for a test file named `my_test.go`) to
make ownership obvious.

Per the [Chromium guidelines for third-party code], place
(appropriately-licensed) data that wasn't created by Chromium developers within
a `third_party` subdirectory under the `data` directory.

[Chromium guidelines for third-party code]: https://chromium.googlesource.com/chromium/src.git/+/master/docs/adding_to_third_party.md

### External data files

Larger data files like audio, video, or graphics files should be stored in
Google Cloud Storage and registered as _external data files_ to avoid
permanently bloating the test repository. A mapping from filenames used during
testing to URLs is stored in `files/external_data.conf` in the bundle package's
directory in the overlay; see e.g. the [external_data.conf file for
tasts-local-tests-cros]. External data files are included in test bundle Portage
packages and also downloaded and pushed to the DUT as needed by `tast run
-build`.

The process for adding an external data file is:

1.  Add a line to the test bundle's `files/external_data.conf` file containing
    the filename that will be used by tests when accessing the file and the
    `gs://` URL where the file is stored.
2.  Update the corresponding ebuild (e.g. [tast-local-tests-cros-9999.ebuild])
    to list the URL in its `TAST_BUNDLE_EXTERNAL_DATA_URLS` variable.
3.  Update the package's `Manifest` file to include the new file's checksums by
    running e.g. `ebuild path/to/tast-local-tests-cros-9999.ebuild manifest` in
    your chroot.
4.  Emerge the package to verify that the new file is downloaded and installed.

> Old versions of external data files should be retained indefinitely in Google
> Cloud Storage so as to not break tests on older system images. Include the
> date as a suffix in the filename to make it easy to add a new version when
> needed, e.g. `my_test_data_20180812.bin`.

[external_data.conf file for tasts-local-tests-cros]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-tests-cros/files/external_data.conf
[tast-local-tests-cros-9999.ebuild]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-tests-cros/tast-local-tests-cros-9999.ebuild

### Internal vs. external

As internal data files are much easier to view and modify than external data
files, it's usually better to check in textual data. Only store binaries as
external data.

### Executables

If your test depends on outside executables, use Portage to build and package
those executables separately and include them in test Chrome OS system images.
Tast [intentionally](design_principles.md) does not support compiling or
deploying other packages that tests depend on.

### Using data files in tests

To register data files (regardless of whether they're checked into the test
repository or stored externally), in your test's `testing.AddTest` call, set the
`testing.Test` struct's `Data` field to contain a slice of data file names
(omitting the `data/` subdirectory):

```go
testing.AddTest(&testing.Test{
	...
	Data: []string{"my_test_data.bin"},
	...
})
```

Later, within the test function, pass the same filename to [testing.State]'s
`DataPath` function to receive the path to the data file on the DUT:

```go
b, err := ioutil.ReadFile(s.DataPath("my_test_data.bin"))
```

See the [example.DataFiles] test for a complete example of using both local and
external data files.

[example.DataFiles]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/example/data_files.go
