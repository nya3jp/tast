# Tast: Writing Tests

[TOC]

## Code location

Public tests are checked into the [tast-tests repository] under the
[src/chromiumos/tast/local/tests/] and [src/chromiumos/tast/remote/tests/]
directories. Tests are grouped into packages by the functionality that they
exercise; for example, the [ui package] contains local tests that exercise
Chrome OS's UI.

Support packages used by tests are located in `local/` and `remote/`, alongside
the `tests/` subdirectories. For example, the [chrome package] can be used by
local tests to interact with Chrome.

## Coding style and best practices

Test code should be formatted by [gofmt] and validated by [go vet]. It should
follow Go's established best practices as described by these documents:

*   [Effective Go]
*   [Go Code Review Comments]

Support packages should be exercised by unit tests when possible. Unit tests can
cover edge cases that may not be typically seen when using the package, and they
greatly aid in future refactorings (since it can be hard to determine the full
set of Tast-based tests that must be run to exercise the package). See [Go's
testing package] for more information about writing tests for Go code. The [Best
practices for writing Chrome OS unit tests] document contains additional
suggestions that may be helpful (despite being C++-centric).

## Naming and defining tests

Tests are identified by names like `ui.ChromeSanity`. The portion before the
period is the test's package name, while the portion after the period is the
function that implements the test. Test function names should follow [Go's
naming conventions], and [acronyms should be fully capitalized]. Test names are
automatically derived and usually shouldn't be specified.

A local test named `example.MyTest` would be placed in a file named
`local/tests/example/my_test.go` (i.e. convert the test name to lowercase and
insert underscores between words) with contents similar to the following:

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

## Reporting errors

The [Tast testing package] (not to be confused with Go's standard `testing`
package) defines a `State` struct that is passed to test functions and used to
report failures and to log messages.

Support packages should not record test failures directly. Instead, return
`error` values and allow tests to decide how to handle them. Support package's
exported functions should typically take [context.Context] arguments and use
them to return early when the test's deadline is reached and to log informative
messages using `testing.ContextLog` and `testing.ContextLogf`.

## Writing output files

Tests can write output files that are automatically copied to the host machine
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

See the `example.DataFiles` test for a complete example of using data files.

## Adding new test packages

When adding a new test package, you must update the test executable's `main.go`
file (either [cmd/local_tests/main.go] or [cmd/remote_tests/main.go]) to
underscore-import the new package so its `init` functions will run and register
tests.

[tast-tests repository]: ../../tast-tests/
[src/chromiumos/tast/local/tests/]: ../../tast-tests/src/chromiumos/tast/local/tests/
[src/chromiumos/tast/remote/tests/]: ../../tast-tests/src/chromiumos/tast/remote/tests/
[ui package]: ../../tast-tests/src/chromiumos/tast/local/tests/ui/
[chrome package]: ../../tast-tests/src/chromiumos/tast/local/chrome/
[gofmt]: https://golang.org/cmd/gofmt/
[go vet]: https://golang.org/cmd/vet/
[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments
[Go's testing package]: https://golang.org/pkg/testing/
[Best practices for writing Chrome OS unit tests]: https://chromium.googlesource.com/chromiumos/docs/+/master/unit_tests.md
[Go's naming conventions]: https://golang.org/doc/effective_go.html#names
[acronyms should be fully capitalized]: https://github.com/golang/go/wiki/CodeReviewComments#initialisms
[Tast testing package]: ../src/chromiumos/tast/testing/
[context.Context]: https://golang.org/pkg/context/
[Running tests]: running_tests.md
[cmd/local_tests/main.go]: ../../tast-tests/src/chromiumos/cmd/local_tests/main.go
[cmd/remote_tests/main.go]: ../../tast-tests/src/chromiumos/cmd/remote_tests/main.go
