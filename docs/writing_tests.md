# Tast: Writing Tests (go/tast-writing)

[TOC]

## Test names

Tests are identified by names like `ui.ChromeLogin`. The portion before the
period is the test's package name, while the portion after the period is the
function that implements the test. Test function names should follow [Go's
naming conventions], and [acronyms should be fully capitalized]. Test names are
automatically derived and usually shouldn't be specified.

## Code location

Public tests built into the default `cros` local and remote test bundles are
checked into the [tast-tests repository] under the
[src/chromiumos/tast/local/bundles/cros/] and
[src/chromiumos/tast/remote/bundles/cros/] directories (which may also be
accessed by the `local_tests` and `remote_tests` symlinks at the top of the
repository). Tests are grouped into packages by the functionality that they
exercise; for example, the [ui package] contains local tests that exercise
Chrome OS's UI.

Support packages used by tests are located in [src/chromiumos/tast/local/] and
[src/chromiumos/tast/remote/], alongside the `bundles/` directories. For
example, the [chrome package] can be used by local tests to interact with
Chrome.

A local test named `ui.MyTest` would be placed in a file named
`src/chromiumos/tast/local/bundles/cros/ui/my_test.go` (i.e. convert the test
name to lowercase and insert underscores between words) with contents similar to
the following:

```go
// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MyTest,
		Desc: "Does X to verify Y",
		Attr: []string{"informational"},
	})
}

func MyTest(s *testing.State) {
	// The actual test goes here.
}
```

Tests may specify [attributes] and [software dependencies] when they are
declared. Setting the `informational` attribute on new tests is recommended, as
they will block the Commit Queue on failure otherwise.

### Adding new test packages

When adding a new test package, you must update the test bundle's `main.go` file
(either [local/bundles/cros/main.go] or [remote/bundles/cros/main.go]) to
underscore-import the new package so its `init` functions will be executed to
register tests.

## Coding style and best practices

Test code should be formatted by [gofmt] and validated by [go vet]. It should
follow Go's established best practices as described by these documents:

*   [Effective Go]
*   [Go Code Review Comments]

Support packages should be exercised by unit tests when possible. Unit tests can
cover edge cases that may not be typically seen when using the package, and they
greatly aid in future refactorings (since it can be hard to determine the full
set of Tast-based tests that must be run to exercise the package). See [Go's
testing package] for more information about writing unit tests for Go code. The
[Best practices for writing Chrome OS unit tests] document contains additional
suggestions that may be helpful (despite being C++-centric).

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

### Startup and shutdown

If a test requires the system to be in a particular state before it runs, it
should include code that tries to get the system into that state if it isn't
there already. Previous tests may have aborted mid-run; it's not safe to make
assumptions that they undid all temporary changes that they made.

Tests should also avoid performing unnecessary de-initialization steps on
completion: UI tests should leave Chrome logged in instead of restarting it, for
example. Since later tests can't safely make assumptions about the initial state
of the system, they'll need to e.g. restart Chrome again regardless, which takes
even more time. In addition to resulting in a faster overall running time for
the suite, leaving the system in a logged-in state makes it easier for
developers to manually inspect it after running the test when diagnosing a
failure.

Note that tests should still undo atypical configuration that leaves the system
in a non-fully-functional state, though. For example, if a test needs to
temporarily stop a service, it should restart it before exiting.

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
services, it's fine to use fine-grained tests.

## Errors and logging

The [Tast testing package] (not to be confused with Go's standard `testing`
package) defines a [State] struct that is passed to test functions and used to
report failures and to log messages.

### How and when

A single test can report multiple errors. Use the `Error` or `Errorf` methods to
report an error and continue or the `Fatal` or `Fatalf` methods to report an
error and halt the test. Respectively, these are analogous to the results of
failed `EXPECT_` and `ASSERT_` macros in [Google Test].

Support packages should not record test failures directly. Instead, return
`error` values and allow tests to decide how to handle them. Support package's
exported functions should typically take [context.Context] arguments and use
them to return an error early when the test's deadline is reached and to log
informative messages using `testing.ContextLog` and `testing.ContextLogf`.

When you're about to do something that could take a while or even hang, log a
message using `Log` or `Logf` first. This both lets developers know what's
happening when they run your test interactively and helps when looking at logs
to investigate timeout failures.

On the other hand, avoid logging unnecessary information that would clutter the
logs. If you want to log a verbose piece of information to help determine the
cause of an error, only do it after the error has occurred.

### Formatting

Please follow [Go's error string conventions] when producing `error` values
(i.e. with `error.New` or `fmt.Errorf`):

> Error strings should not be capitalized (unless beginning with proper nouns or
> acronyms) or end with punctuation, since they are usually printed following
> other context.

When adding detail to an existing error, append the existing error to the new
string:

```go
if err := doSomething(id); err != nil {
	return fmt.Errorf("doing something to %q failed: %v", id, err)
}
```

Tast log messages and error reasons should be capitalized phrases without any
trailing punctuation:

```go
s.Log("Asking Chrome to log in")
...
if err != nil {
	s.Fatal("Failed to log in: ", err)
}
s.Logf("Logged in as user %q with ID %v", user, id)
```

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

## Data files

Tests can register ancillary data files that will be copied to the DUT and made
available while the test is running; consider a short binary audio file that a
test plays in a loop, for example.

### Internal data files

Small non-binary data files should be checked into a `data/` subdirectory under
the test package as _internal data files_. Prefix their names by the test file's
name (e.g. `data/my_test_some_data.txt` for a test file named `my_test.go`) to
make ownership obvious.

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

### Executables

If your test depends on outside executables, use Portage to build and package
those executables separately and include them in test Chrome OS system images.

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

Later, within the test function, pass the same filename to `testing.State`'s
`DataPath` function to receive the path to the data file on the DUT:

```go
b, err := ioutil.ReadFile(s.DataPath("my_test_data.bin"))
```

See the [example.DataFiles] test for a complete example of using both local and
external data files.

[Go's naming conventions]: https://golang.org/doc/effective_go.html#names
[acronyms should be fully capitalized]: https://github.com/golang/go/wiki/CodeReviewComments#initialisms
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[src/chromiumos/tast/local/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/
[src/chromiumos/tast/remote/bundles/cros/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/bundles/cros/
[ui package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/ui/
[src/chromiumos/tast/local/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/
[src/chromiumos/tast/remote/]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/
[chrome package]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/chrome/
[attributes]: test_attributes.md
[software dependencies]: test_dependencies.md
[local/bundles/cros/main.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/main.go
[remote/bundles/cros/main.go]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/remote/bundles/cros/main.go
[gofmt]: https://golang.org/cmd/gofmt/
[go vet]: https://golang.org/cmd/vet/
[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments
[Go's testing package]: https://golang.org/pkg/testing/
[Best practices for writing Chrome OS unit tests]: https://chromium.googlesource.com/chromiumos/docs/+/master/unit_tests.md
[scoped at the package level]: https://golang.org/ref/spec#Declarations_and_scope
[chromecrash]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chromecrash/
[ui.ChromeCrashLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chrome_crash_logged_in.go
[ui.ChromeCrashNotLoggedIn]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/master/src/chromiumos/tast/local/bundles/cros/ui/chrome_crash_not_logged_in.go
[TotT 227]: http://go/tott/227
[TotT 324]: http://go/tott/324
[TotT 339]: http://go/tott/339
[TotT 520]: http://go/tott/520
[Unit Testing Best Practices Do's and Don'ts]: http://go/unit-test-practices#behavior-testing-dos-and-donts
[Tast testing package]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/testing/
[State]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State
[Google Test]: https://github.com/google/googletest
[context.Context]: https://golang.org/pkg/context/
[Go's error string conventions]: https://github.com/golang/go/wiki/CodeReviewComments#error-strings
[Running tests]: running_tests.md
[external_data.conf file for tasts-local-tests-cros]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-tests-cros/files/external_data.conf
[tast-local-tests-cros-9999.ebuild]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-tests-cros/tast-local-tests-cros-9999.ebuild
[example.DataFiles]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/example/data_files.go
