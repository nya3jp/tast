# Tast: Writing Tests

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
		Attr: []string{"experimental"},
	})
}

func MyTest(s *testing.State) {
	// The actual test goes here.
}
```

Tests may specify [attributes] and [software dependencies] when they are
declared.

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
within your test function:

```go
func MyTest(s *testing.State) {
	const (
		someValue = "foo"
	)
	dataMap := map[string]int{"a": 1, "b": 2}
	...
}
```

If you need to share functionality between tests in the same package, please
introduce a new descriptively-named subpackage; see e.g. the [chromecrash]
package within the `ui` package, used by the [ui.ChromeCrashLoggedIn] and
[ui.ChromeCrashNotLoggedIn] tests.

## Errors and logging

The [Tast testing package] (not to be confused with Go's standard `testing`
package) defines a [State] struct that is passed to test functions and used to
report failures and to log messages.

### How and when

A single test can report multiple errors. Use the `Error` or `Errorf` methods to
report an error and continue or the `Fatal` or `Fatalf` methods to report an
error and halt the test.

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

## Writing output files

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

## Using data files

Tests can register ancillary data files that will be copied to the DUT and made
available while the test is running; consider a short binary audio file that a
test plays in a loop, for example.

Data files should be checked into a `data/` subdirectory under the test package.
Prefix their names by the test file's name (e.g.
`data/audio_playback_sample.wav` for a test file named `audio_playback.go`) to
make ownership obvious.

Keep data files minimal; source control systems are not adept at managing large
binary files. If your test depends on outside executables, use Portage to build
and package those executables separately and include them in test Chrome OS
system images.

To register data files, in your test's `testing.AddTest` call, set the
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

See the [example.DataFiles] test for a complete example of using data files.

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
[Tast testing package]: https://chromium.googlesource.com/chromiumos/platform/tast/+/master/src/chromiumos/tast/testing/
[State]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State
[context.Context]: https://golang.org/pkg/context/
[Go's error string conventions]: https://github.com/golang/go/wiki/CodeReviewComments#error-strings
[Running tests]: running_tests.md
[example.DataFiles]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros/example/data_files.go
