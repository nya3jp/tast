# Tast Codelab #1: platform.DateFormat (go/tast-codelab-1)

This codelab walks through the creation of a short Tast test that checks that
the `date` command prints the expected output when executed with various
arguments. In doing so, we'll learn:

*   how to define new tests
*   how to test multiple cases without repeating code
*   how to run external commands
*   how to report errors

We probably wouldn't want to actually test this, since the `date` command is
likely very stable by this point and any regressions in it would hopefully be
caught long before reaching Chrome OS. Since there's a cost in writing, running,
and maintaining tests, we want to focus on areas where we'll get the most
benefit.

## Boring boilerplate

To start out, we'll create a file at
`src/platform/tast-tests/src/chromiumos/tast/local/bundles/cros/platform/date_format.go`
containing the standard copyright header, the name of the package that this file
belongs to, and an `import` block listing the packages that we're using:

```go
// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testing"
)
```

To keep tests from becoming hard to find, we favor using one of the existing
[test category packages] in the `cros` local test bundle; `platform` seems like
a good fit. If we were to add many more tests for the `date` command later, it
would be easy to introduce a new `date` package for them then.

If you configure your editor to run [goimports] whenever you save Go files, then
you generally don't need to worry about managing imports of standard Go
packages, but you'll still need to add Tast-specific dependencies (i.e. packages
beginning with `chromiumos/`) yourself.

[test category packages]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/chromiumos/tast/local/bundles/cros
[goimports]: https://godoc.org/golang.org/x/tools/cmd/goimports

## Test metadata

Next, we add an `init` function containing a single [testing.AddTest] call that
registers our test:

```go
func init() {
	testing.AddTest(&testing.Test{
		Func: DateFormat,
		Desc: "Checks that the date command prints dates as expected",
		Contacts: []string{
			"me@chromium.org",         // Test author
			"tast-users@chromium.org", // Backup mailing list
		},
		Attr: []string{"group:mainline", "informational"},
	})
}
```

`init` functions run automatically before all other code in the package. We pass
a pointer to a [testing.Test] struct to `testing.AddTest`; this contains our
test's metadata.

The `Func` field contains the main test function that we'll define, i.e. the
entry point into the test. The function's name is also used to derive the test's
name; since our test is in the `platform` package, it will be named
`platform.DateFormat`. We don't include words like `Check`, `Test`, or `Verify`
the test's name: we already know that it's a test, after all.

`Desc` is a short, human-readable phrase describing the test, and `Contacts`
lists the email addresses of people and mailing lists that are responsible for
the test.

`Attr` contains free-form strings naming this test's [attributes].
`group:mainline` indicates that this test is in [the mainline group], the
default group for functional tests. `informational` indicates that this test is
non-critical, i.e. it won't run on the Chrome OS Commit Queue or on the
Pre-Flight Queue (PFQ) builders that are used to integrate new versions of
Chrome or Android into the OS. All [new mainline tests] (internal link) should
start out with the `informational` attribute until they've been proven to be
stable.

[testing.AddTest]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#AddTest
[testing.Test]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Test
[attributes]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md
[the mainline group]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_attributes.md
[new mainline tests]: https://chrome-internal.googlesource.com/chromeos/chromeos-admin/+/HEAD/doc/tast_add_test.md

## Test function

Next comes the signature of our main `DateFormat` test function:

```go
func DateFormat(ctx context.Context, s *testing.State) {
```

All Tast test functions receive [context.Context] and [testing.State] arguments;
by convention, these are named `ctx` and `s`. The `Context` is used primarily to
carry a deadline that represents the test's timeout, while the `State` is used
to fetch test-related information at runtime and to report log messages or
errors.

[context.Context]: https://golang.org/pkg/context/
[testing.State]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State

## Test cases

Within the test function, we need to run the `date` command with various
arguments and compare its actual output against the corresponding expected
output. One way to do this would be be repeating code for each test case, but
that would result in a lot of duplication and make future changes harder. A
common approach used in Go code for cases like this is to iterate over an array
of anonymous structures, each of which contains a test case:

```go
	for _, tc := range []struct {
		date string // value to pass via --date flag
		spec string // spec to pass in "+"-prefixed arg
		exp  string // expected UTC output (minus trailing newline)
	}{
		{"2004-02-29 16:21:42 +0100", "%Y-%m-%d %H:%M:%S", "2004-02-29 15:21:42"},
		{"Sun, 29 Feb 2004 16:21:42 -0800", "%Y-%m-%d %H:%M:%S", "2004-03-01 00:21:42"},
	} {
		// Test body will go here.
	}
```

The syntax can be confusing at first, so let's break it down.

First, we start a `for` loop. We use the double-assignment form of `range`,
which provides an index and a value for each element in a slice. We're not
interested in the index, so we ignore it (by assigning to underscore) and copy
each element to a `tc` (for "test case") value. There's a convention in Go code
to use [short names] for variables that have a limited scope, like this one.

The next part of the loop construct explains what we're iterating over: a slice
of structs, each of which contains three string fields. We document each field's
purpose using an end-of-line comment. Single-line or multi-line comments
typically consist of full sentences, but it's fine to use phrases for short
end-of-line comments like these.

If we were going to use this struct multiple times, we would give it a name
using a `type` declaration, but since we're only using it within the loop here,
it's simpler to keep it anonymous.

Next, we provide the slice's values: a comma-separated list of struct literals.
Since we're providing all of the struct fields, we can omit the field names.

Finally, we provide a block containing the loop body. We'll discuss that in the
next section.

[short names]: https://talks.golang.org/2014/names.slide

## Loop body

```go
		cmd := testexec.CommandContext(ctx, "date", "--utc", "--date="+tc.date, "+"+tc.spec)
		if out, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			s.Errorf("%q failed: %v", shutil.EscapeSlice(cmd.Args), err)
		} else if outs := strings.TrimRight(string(out), "\n"); outs != tc.exp {
			s.Errorf("%q printed %q; want %q", shutil.EscapeSlice(cmd.Args), outs, tc.exp)
		}
```

First, we declare a `testexec.Cmd` named `cmd` that will be used to execute the
`date` command with the appropriate arguments for this test case. The [testexec]
package is similar to the standard [exec] package but provides a few
Tast-specific niceties.

After that, we run the command synchronously to completion using `Output`,
getting back its stdout as a `[]byte`, along with an `error` value that is
non-nil if the process didn't run successfully. We pass
`testexec.DumpLogToError`, which is an option instructing the `Cmd` to log
likely-useful information like stderr if the process fails.

We use the assignment form of `if`, which lets us perform an assignment before
testing a boolean condition. If a non-nil `error` was returned, then we report a
test error. We use `Errorf` so we can provide a `printf`-like format string, and
we include both the quoted command and the error that was returned. The
`Something failed: <error with more details>` form is recommended for error
messages in Tast tests for consistency.

Finally, we add an `else if` that calls `strings.TrimRight` to trim a trailing
newline from `out` (which is still in scope here), and compare the resulting
string against the test case's expected output. If they don't match, then we
report another error using `Errorf`.

In the error messages above, we use the [shutil] package to escape the command
that we ran so it's easier to copy-and-paste to run manually. Since we're
logging strings that were produced by an outside command and that may contain
spaces, we use `%q` so they'll be quoted automatically. The `<foo>
printed/produced/= <bar>; want <baz>` form is also common in Go unit tests and
recommended in Tast.

[testexec]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/testexec
[exec]: https://golang.org/pkg/os/exec/
[shutil]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/shutil

## Wrapping it up

After closing our loop and the test function, we're done!

```go
	}
}
```

If the test reports one or more errors in the loop, it fails. If no errors have
been reported by the time that the test function returns, then the test passes.

The test can be run using a command like `tast -verbose run <DUT>
platform.DateFormat`. See the [Running Tests] document for more information.

If you want to see how a more-complicated test is written, check out
[Codelab #2].

[Running Tests]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/running_tests.md
[Codelab #2]: codelab_2.md

## Full code

Here's a full listing of the test's code:

```go
// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DateFormat,
		Desc: "Checks that the date command prints dates as expected",
		Contacts: []string{
			"me@chromium.org",         // Test author
			"tast-users@chromium.org", // Backup mailing list
		},
		Attr: []string{"group:mainline", "informational"},
	})
}

func DateFormat(ctx context.Context, s *testing.State) {
	for _, tc := range []struct {
		date string // value to pass via --date flag
		spec string // spec to pass in "+"-prefixed arg
		exp  string // expected UTC output (minus trailing newline)
	}{
		{"2004-02-29 16:21:42 +0100", "%Y-%m-%d %H:%M:%S", "2004-02-29 15:21:42"},
		{"Sun, 29 Feb 2004 16:21:42 -0800", "%Y-%m-%d %H:%M:%S", "2004-03-01 00:21:42"},
	} {
		cmd := testexec.CommandContext(ctx, "date", "--utc", "--date="+tc.date, "+"+tc.spec)
		if out, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			s.Errorf("%q failed: %v", shutil.EscapeSlice(cmd.Args), err)
		} else if outs := strings.TrimRight(string(out), "\n"); outs != tc.exp {
			s.Errorf("%q printed %q; want %q", shutil.EscapeSlice(cmd.Args), outs, tc.exp)
		}
	}
}
```
