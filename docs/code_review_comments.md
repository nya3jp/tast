# Tast: Code Review Comments (go/tast-code-review-comments)

This document collects common comments made during reviews of Tast tests.

Tast code should also follow Go's established best practices as described in
[Effective Go] and [Go Code Review Comments].  
[Go Style] (internal document similar to public style guides) is also a good
read and source of style suggestions.

[Effective Go]: https://golang.org/doc/effective_go.html
[Go Code Review Comments]: https://github.com/golang/go/wiki/CodeReviewComments
[Go Style]: http://go/go-style

[TOC]


## CombinedOutput

In general you should not parse the result of [`CombinedOutput`].
Its result interleaves stdout and stderr, so parsing it is very likely flaky.

If the message you are concerned with is written to stdout, use [`Output`] instead.
In the case of stderr, capture it explicitly with [`Run`].

```go
// GOOD
out, err := testexec.CommandContext(...).Output()
if regexp.Match(out, "...") { ... }
```

```go
// GOOD
cmd := testexec.CommandContext(...)
var stderr bytes.Buffer
cmd.Stderr = &stderr
if err := cmd.Run(...); err != nil { ... }
out := stderr.Bytes()
if regexp.Match(out, "...") { ... }
```

```go
// BAD
out, err := testexec.CommandContext(...).CombinedOutput()
if regexp.Match(out, "...") { ... }
```

[`CombinedOutput`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/testexec#Cmd.CombinedOutput
[`Output`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/testexec#Cmd.Output
[`Run`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast-tests.git/src/chromiumos/tast/common/testexec#Cmd.Run


## Context cancellation

After calling functions to create a new context.Context with a new deadline
(e.g. [`ctxutil.Shorten`], [`context.WithTimeout`] etc.), always call the cancel
function with a defer statement. In many cases those functions start a new
goroutine associated with the new context, and it is released only on
cancellation or expiration of the context. Thus failing to cancel the context
may lead to resource leaks.

```go
// GOOD
ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
defer cancel()
```

```go
// BAD
ctx, _ = ctxutil.Shorten(ctx, 5*time.Second)
```

[This article](https://developer.squareup.com/blog/always-be-closing/)
describes how a similar bug caused massive production issues at Square.

[`ctxutil.Shorten`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/ctxutil#Shorten
[`context.WithTimeout`]: https://godoc.org/context#WithTimeout


## Context timeout

[`context.Context`] carries the deadline of a function call. Functions that may
block should take [`context.Context`] as an argument and honor its deadline.

```go
// GOOD
func httpGet(ctx context.Context, url string) ([]byte, error) { ... }
```

```go
// BAD
func httpGet(url string) ([]byte, error) { ... }
```

[`context.Context`]: https://godoc.org/context


## Fixtures

Whenever possible, use [fixtures] rather than calling setup functions by
yourself. Fixtures speeds up testing when multiple tests sharing the same
fixtures are run at a time (e.g. in the commit queue).

```go
// GOOD
func init() {
	testing.AddTest(&testing.Test{
		Func: Example,
		Fixture: "chromeLoggedIn",
		...
	})
}
```

```go
// BAD
func Example(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	...
}
```

See also [a section in the Writing Tests document](writing_tests.md#Fixtures).

[fixtures]: writing_tests.md#Fixtures


## Sleep

Sleeping without polling for a condition is discouraged, since it makes tests
flakier (when the sleep duration isn't long enough) or slower (when the duration
is too long).

The [`testing.Poll`] function makes it easier to honor timeouts while polling
for a condition:

```go
// GOOD
startServer()
if err := testing.Poll(ctx, func (ctx context.Context) error {
	return checkServerReady()
}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
	return errors.Wrap(err, "server failed to start")
}
```

```go
// BAD
startServer()
testing.Sleep(ctx, 10*time.Second) // hope that the server starts in 10 seconds
```

If you really need to sleep for a fixed amount of time, use [`testing.Sleep`] to
honor the context timeout.

[`testing.Poll`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Poll
[`testing.PollBreak`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#PollBreak
[`testing.Sleep`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#Sleep


## State

[`testing.State`] implements a lot of methods allowing tests to access all the
information and utilities they may use to perform testing. Since it contains
many things, passing it to a function as an argument makes it difficult to
tell what are inputs and outputs of the function from its signature. Also,
such a function cannot be called from [gRPC services].

For this reason, `tast-lint` forbids use of [`testing.State`] in support
library packages. Other packages, such as test main functions and test
subpackages, can still use [`testing.State`], but think carefully if you really
need it.

In many cases you can avoid passing [`testing.State`] to a function:

*   If a function needs to report an error, just return an `error` value,
    and call [`testing.State.Error`] (or its family) at the highest possible
    level in the call stack.
*   If a function needs to log its progress, pass a [`context.Context`] so it
    can call [`testing.ContextLog`].
*   If a function needs to write to the output directory, just pass the path
    returned by [`testing.State.OutDir`]. Alternatively you can pass a
    [`context.Context`] and call [`testing.ContextOutDir`].

[gRPC services]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Remote-procedure-calls-with-gRPC
[`testing.State`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State
[`testing.State.Error`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State.Error
[`context.Context`]: https://godoc.org/context
[`testing.ContextLog`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#ContextLog
[`testing.State.OutDir`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#State.OutDir
[`testing.ContextOutDir`]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#ContextOutDir
[the allowlist in tast-lint]: https://chromium.googlesource.com/chromiumos/platform/tast/+/refs/heads/main/src/chromiumos/tast/cmd/tast-lint/check/disallow_testingstate.go


## Test dependencies

Avoid skipping tests or subtests by checking device capabilities at runtime.
Instead specify [software/hardware dependencies] to declare software features
and/or hardware capabilities your test depends on.

```go
// GOOD
func init() {
	testing.AddTest(&testing.Test{
		Func: CheckCamera,
		SoftwareDeps: []string{"camera_720p", "chrome"},
		...
	})
}
```

```go
// BAD
func CheckCamera(ctx context.Context, s *testing.State) {
	if !supports720PCamera() {
		s.Log("Skipping test; device unsupported")
		return
	}
	...
}
```

See also [a section in the Writing Tests document](writing_tests.md#device-dependencies).

[software/hardware dependencies]: test_dependencies.md
