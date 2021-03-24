# Tast Codelab #2: kernel.LogMount (go/tast-codelab-2)

> This document assumes that you've already gone through [Codelab #1].

This codelab follows the creation of a Tast test that verifies that the kernel
logs a message to its ring buffer when a filesystem is mounted. In doing so,
we'll learn about the following:

*   creating new contexts to limit the running time of operations
*   declaring separate helper functions
*   deciding when to run external commands vs. using standard Go code
*   using channels to communicate between goroutines
*   waiting for asynchronous events
*   cleaning up after ourselves

## Getting started

Before writing any code, let's think about the basic approach that we should use
to test this. We can create a filesystem within an on-disk file and then use a
[loopback mount]. When the filesystem is mounted, the kernel will asynchronously
write a message like this to its ring buffer:

```
[124273.844282] EXT4-fs (loop4): mounted filesystem without journal. Opts: (null)
```

The kernel ring buffer can be viewed using the [dmesg] command. One option would
be to mount the filesystem, sleep a bit, and then run `dmesg` and look for the
expected message, but that approach is problematic for multiple reasons:

1.  The kernel might take longer than expected to log the message, resulting in
    our test being flaky.
2.  If other messages are being logged quickly while our test is running, the
    `mounted filesystem` message might be pushed out of the buffer before we
    check it, resulting in our test being flaky.
3.  The ring buffer may already contain a `mounted filesystem` message from an
    earlier mount, resulting in our test not detecting failures.

Luckily, in Linux 3.5.0 and later, `dmesg --follow` can be used to tail new log
messages. If we start `dmesg --follow` before mounting the filesystem, that
resolves the first two concerns above. For the third concern, we can use `dmesg
--clear` to clear the buffer beforehand — that doesn't completely eliminate the
risk of seeing a stale message, but it at least makes it less likely.

We also need to think about how we're going to consume `dmesg --follow`'s output
in our test. Go doesn't provide much standard support for non-blocking I/O, but
it does make it easy to run code concurrently in a separate [goroutine] and pass
data between goroutines using [channels]. (There are more helpful links about
concurrency in Go in the [Concurrency] section of the [Writing Tests] doc.) We
can start a goroutine that reads the output from the `dmesg` process as long as
it's running, and writes each line to a channel that's read by the main
goroutine. [select] then lets us to perform [non-blocking operations on
channels].

Now that we've thought through how the test should work, let's start writing
some code.

[Codelab #1]: codelab_1.md
[loopback mount]: https://en.wikipedia.org/wiki/Loop_device
[dmesg]: https://linux.die.net/man/1/dmesg
[goroutine]: https://tour.golang.org/concurrency/1
[channels]: https://tour.golang.org/concurrency/2
[Concurrency]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Concurrency
[Writing Tests]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md
[select]: https://tour.golang.org/concurrency/5
[non-blocking operations on channels]: https://gobyexample.com/non-blocking-channel-operations

## Test setup

We'll skip over boilerplate like import statements and the test metadata since
those are already covered by [Codelab #1], but you can see these portions of the
test in the full listing at the bottom of this document. Instead, let's start
with the beginning of our test function:

```go
func LogMount(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	shortCtx, shortCancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer shortCancel()
```

When the test exits, we need to perform various cleanup tasks. `defer` is the
standard way of doing this. Some of this cleanup, like running the `umount`
command, may require a context, though — if the test times out, the original
context will be expired by the time that the deferred functions are executed,
resulting in them not working correctly. A common way to resolve this is by
deriving a new context with a slightly-shorter deadline from the original one
that was passed to the test function. Then, we can use the shorter context
throughout the test while saving the longer original context for cleanup. The
[ctxutil.Shorten] function makes it easy to derive a new context with a shorter
deadline.

We also create a temporary directory that we'll use to hold both the filesystem
and the mount point that we use for it:

```go
	// Create a temp dir in /tmp to ensure that we don't leave stale mounts in
	// Tast's temp dir if we're interrupted.
	td, err := ioutil.TempDir("/tmp", "tast.kernel.LogMount.")
	if err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	defer os.RemoveAll(td)
```

We explicitly ask for the temp dir to be created under `/tmp`. Tast sets
`TMPDIR` explicitly for tests, and if we're interrupted before we can unmount
the filesystem, leaving a mount in `TMPDIR` can cause problems when Tast tries
to clean it up later.

[ctxutil.Shorten]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/ctxutil#Shorten

## Creating the filesystem

Next, we need to create the on-disk filesystem so we can mount it. Since we only
need to do this once, we could just put the code in the main test function.
However, arguments can be made for declaring a separate helper function:

*   `defer` can be used to run code when the helper function returns.
*   Creating a filesystem requires many steps and is a self-contained operation,
    so declaring a helper function may make the main test function more
    readable.

Go allows top-level identifiers to be declared in any order, so we'll put this
helper at the end of the file, after the main test function. This makes it
easier for readers to see the big picture without getting bogged down in the
details.

Top-level functions, variables, and constants are shared across all tests in the
`kernel` package, so we'll also use a descriptive name that's hopefully unlikely
to conflict with other tests:

```go
// makeFilesystem creates an ext4 filesystem of the requested size (in bytes) at path p.
func makeFilesystem(ctx context.Context, p string, size int64) error {
```

This function's signature deserves some discussion. It takes a context, which is
needed for running the `mkfs.ext4` command, and it returns an `error`. Instead
of returning an `error`, we could have made it take a `*testing.State` argument
and report errors directly using the `Fatal` method, but there are reasons to
avoid this:

*   `testing.State` carries a lot of additional information that this function
    doesn't need. It's often better to practice [information hiding] and give a
    function only the information that it needs.
*   If errors are reported directly using `testing.State`'s `Error`, `Fatal`,
    etc. methods, the caller loses the ability to decide how to deal with
    errors.
*   When errors are reported directly by helper functions, the overall flow of
    the test becomes harder to see. Every call to a helper function could
    potentially result in a fatal test error.

As a result, we choose to instead return an `error` value from this function.

Next, we need to create a new file:

```go
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	// Clean up if we get an error mid-initialization.
	toClose := f
	defer func() {
		if toClose != nil {
			toClose.Close()
		}
	}()
```

The `toClose` variable is also worth discussing. The usual pattern in Go is to
create an [os.File] and then immediately defer a call to its `Close` method.
Here, we  want to use `f` to write to the file but then close it before running
`mkfs.ext4` to initialize the filesystem. Various things can go wrong while
we're writing the file, though, and we don't want to need to remember to call
`f.Close` before returning in each of those cases.

To support automatically closing `f` if an error is encountered while still
being able to manually close it midway through the function, we declare a
`toClose` variable and defer a function that will close it if it's non-`nil`. If
we end up closing `f` manually, we can set `toClose` to `nil` so that the
deferred function becomes a no-op. This pattern is useful when performing
initialization that requires multiple steps: if initialization is interrupted,
cleanup will be automatically performed.

```go
	// Seek to the end of the requested size and write a byte.
	if _, err := f.Seek(size-1, 0); err != nil {
		return err
	}
	if _, err := f.Write([]byte{0xff}); err != nil {
		return err
	}
	toClose = nil // disarm cleanup
	if err := f.Close(); err != nil {
		return err
	}

	return testexec.CommandContext(ctx, "mkfs.ext4", p).Run(testexec.DumpLogOnError)
}
```

The final thing of note in this function is the use of standard Go code like
`os.Create`, `f.Seek`, and `f.Write` instead of e.g. calling the `dd` command to
write a file. When the work being performed is straightforward, standard code
is usually preferable, as it both makes it easier to see the exact operations
that are being performed and provides vastly better error-reporting when
something goes wrong.

[information hiding]: https://en.wikipedia.org/wiki/Information_hiding
[os.File]: https://golang.org/pkg/os/#File

## Starting dmesg

Let's also create a helper function that starts the `dmesg` process and reads
its output:

```go
// streamDmesg clears the kernel ring buffer and then starts a dmesg process and
// asynchronously copies all log messages to a channel. The caller is responsible
// for killing and waiting on the returned process.
func streamDmesg(ctx context.Context) (*testexec.Cmd, <-chan string, error) {
```

This function returns a handle that should be used to kill and clean up the
already-started `dmesg` process, a [unidirectional channel] for reading log
messages, and an error. We don't [name the return arguments] since their purpose
seems clear, but we still document what the function does, and (most
importantly) the contract that the caller is responsible for stopping the
process.

Next, we'll run `dmesg --clear` synchronously to get rid of old messages and
then start `dmesg --follow` asychronously with its stdout automatically copied
to a pipe named `stdout`:

```go
	// Clear the buffer first so we don't see stale messages.
	if err := testexec.CommandContext(ctx, "dmesg", "--clear").Run(
		testexec.DumpLogOnError); err != nil {
		return nil, nil, errors.Wrap(err, "failed to clear log buffer")
	}

	// Start a dmesg process that writes messages to stdout as they're logged.
	cmd := testexec.CommandContext(ctx, "dmesg", "--follow")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start dmesg")
	}
```

[errors.Wrap] is used to attach additional context to some of the returned
errors.

Note the difference in formatting between the `error` values that we're
returning here and the `s.Fatal` message that appeared in the main test
function. Go `error` values should always contain lowercase phrases without any
punctuation, while it's the convention in Tast for logging and error messages
(i.e. those written via `testing.State` or [testing.ContextLog]) to be
capitalized, again without any punctuation. See the [Formatting] section of the
[Writing Tests] document for more discussion.

Finally, we need to read the output from `dmesg`:

```go
	// Start a goroutine that just passes lines from dmesg to a channel.
	ch := make(chan string)
	go func() {
		defer close(ch)

		// Writes msg to ch and returns true if more messages should be written.
		writeMsg := func(msg string) bool {
			// To avoid blocking forever on a write to ch if nobody's reading from
			// it, we use a non-blocking write. If the channel isn't writable, sleep
			// briefly and then check if the context's deadline has been reached.
			for {
				if ctx.Err() != nil {
					return false
				}

				select {
				case ch <- msg:
					return true
				default:
					testing.Sleep(ctx, 10*time.Millisecond)
				}
			}
		}

		// The Scan method will return false once the dmesg process is killed.
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if !writeMsg(sc.Text()) {
				break
			}
		}
		// Don't bother checking sc.Err(). The test will already fail if the expected
		// message isn't seen.
	}()

	return cmd, ch, nil
}
```

We create a channel for passing log messages and then start a goroutine that
uses [bufio.Scanner] to read from the stdout pipe one line at a time. We loop
over the lines and copy each to the channel.

The channel is unbuffered, so writes to it will block if there isn't a reader on
the other end. The reader in this scenario is the main test function, and we
don't want our goroutine to block indefinitely if the test exits before it's
read everything that we're trying to write to the channel.

To handle this case, we define a nested `writeMsg` function that performs the
write in a `select` statement with a `default` case that will be executed
immediately if the channel isn't currently writable. If we hit that, we sleep
briefly (to avoid burning CPU cycles unnecessarily), check whether the context's
deadline has been reached (indicating that the test completed), and then try to
write to the channel again.

[unidirectional channel]: https://gobyexample.com/channel-directions
[name the return arguments]: https://github.com/golang/go/wiki/CodeReviewComments#named-result-parameters
[testing.ContextLog]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/testing#ContextLog
[errors.Wrap]: https://godoc.org/chromium.googlesource.com/chromiumos/platform/tast.git/src/chromiumos/tast/errors#Wrap
[Formatting]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Formatting
[bufio.Scanner]: https://golang.org/pkg/bufio/#Scanner

## Finishing the test

Let's go back to the main test function now and make use of the helper functions
that we just added. First, we'll create a 1-megabyte filesystem named `fs.bin`
in the temporary directory that we created earlier:

```go
	src := filepath.Join(td, "fs.bin")
	s.Log("Creating filesystem at ", src)
	if err := makeFilesystem(shortCtx, src, 1024*1024); err != nil {
		s.Fatal("Failed creating filesystem: ", err)
	}
```

We're using `s.Log` to communicate what we're going to do before doing it, which
is helpful both for people reading the code (i.e. these messages can take the
place of comments) and for people looking at test logs. It's especially useful
to log before performing an operation that could take a while to complete, as
these messages are essential for investigating timeouts.

Next, we start the `dmesg --follow` process:

```go
	s.Log("Starting dmesg")
	dmesgCmd, dmesgCh, err := streamDmesg(shortCtx)
	if err != nil {
		s.Fatal("Failed to start dmesg: ", err)
	}
	defer dmesgCmd.Wait()
	defer dmesgCmd.Kill()
```

We defer `Wait` and `Kill` calls to ensure that the process is stopped when the
main test function returns. `defer` statements are executed in last-in,
first-out order, so these calls will send `SIGKILL` to the process before
waiting on its exit status.

Now that we're reading log messages from the kernel, we can create a mount point
and mount the filesystem:

```go
	dst := filepath.Join(td, "mnt")
	s.Logf("Mounting %v at %v", src, dst)
	if err := os.Mkdir(dst, 0755); err != nil {
		s.Fatal("Failed to create mount point: ", err)
	}
	if err := testexec.CommandContext(shortCtx, "mount", "-o", "loop", src, dst).Run(); err != nil {
		s.Fatal("Failed to mount filesystem: ", err)
	}
	defer func() {
		s.Log("Unmounting ", dst)
		if err := testexec.CommandContext(ctx, "umount", "-f", dst).Run(); err != nil {
			s.Error("Failed to unmount filesystem: ", err)
		}
	}()
```

Immediately after the `mount` command succeeds using the shortened context, we
defer a function that will run `umount` using the full-length context.

Finally, we need to wait for the expected log message to show up on the channel
that the goroutine started by `streamDmesg` is writing to. We expect the message
to be logged reasonably quickly, so we derive a new context with a 15-second
timeout:

```go
	// The message shouldn't take long to show up, so derive a short context for it.
	watchCtx, watchCancel := context.WithTimeout(shortCtx, 15*time.Second)
	defer watchCancel()
```

Now we just need to see the `mounted filesystem` message from the kernel. We log
a message to make it clear what we're doing and again use `select` to avoid
blocking indefinitely:

```go
	// We expect to see a message like "[124273.844282] EXT4-fs (loop4): mounted filesystem without journal. Opts: (null)".
	const expMsg = "mounted filesystem"
	s.Logf("Watching for %q in dmesg output", expMsg)
WatchLoop:
	for {
		select {
		case msg := <-dmesgCh:
			s.Logf("Got message %q", msg)
			if strings.Contains(msg, expMsg) {
				break WatchLoop
			}
		case <-watchCtx.Done():
			s.Fatalf("Didn't see %q in dmesg: %v", expMsg, watchCtx.Err())
		}
	}
}
```

This time, we have a loop that repeatedly waits either for `dmesgCh` to become
readable or for `watchCtx` to expire (indicating that 15 seconds have passed).
If we see the expected message, we break out of the loop using a `WatchLoop`
[label] that we defined earlier.

[label]: https://golang.org/ref/spec#Break_statements

## Full code

Here's a full listing of the test's code:

```go
// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"bufio"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     LogMount,
		Desc:     "Checks that the kernel logs a message when a filesystem is mounted",
		Contacts: []string{"me@chromium.org", "tast-users@chromium.org"},
		Attr:     []string{"group:mainline", "informational"},
	})
}

func LogMount(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	shortCtx, shortCancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer shortCancel()

	// Create a temp dir in /tmp to ensure that we don't leave stale mounts in
	// Tast's temp dir if we're interrupted.
	td, err := ioutil.TempDir("/tmp", "tast.kernel.LogMount.")
	if err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	defer os.RemoveAll(td)

	src := filepath.Join(td, "fs.bin")
	s.Log("Creating filesystem at ", src)
	if err := makeFilesystem(shortCtx, src, 1024*1024); err != nil {
		s.Fatal("Failed creating filesystem: ", err)
	}

	s.Log("Starting dmesg")
	dmesgCmd, dmesgCh, err := streamDmesg(shortCtx)
	if err != nil {
		s.Fatal("Failed to start dmesg: ", err)
	}
	defer dmesgCmd.Wait()
	defer dmesgCmd.Kill()

	dst := filepath.Join(td, "mnt")
	s.Logf("Mounting %v at %v", src, dst)
	if err := os.Mkdir(dst, 0755); err != nil {
		s.Fatal("Failed to create mount point: ", err)
	}
	if err := testexec.CommandContext(shortCtx, "mount", "-o", "loop", src, dst).Run(); err != nil {
		s.Fatal("Failed to mount filesystem: ", err)
	}
	defer func() {
		s.Log("Unmounting ", dst)
		if err := testexec.CommandContext(ctx, "umount", "-f", dst).Run(); err != nil {
			s.Error("Failed to unmount filesystem: ", err)
		}
	}()

	// The message shouldn't take long to show up, so derive a short context for it.
	watchCtx, watchCancel := context.WithTimeout(shortCtx, 15*time.Second)
	defer watchCancel()

	// We expect to see a message like "[124273.844282] EXT4-fs (loop4): mounted filesystem without journal. Opts: (null)".
	const expMsg = "mounted filesystem"
	s.Logf("Watching for %q in dmesg output", expMsg)
WatchLoop:
	for {
		select {
		case msg := <-dmesgCh:
			s.Logf("Got message %q", msg)
			if strings.Contains(msg, expMsg) {
				break WatchLoop
			}
		case <-watchCtx.Done():
			s.Fatalf("Didn't see %q in dmesg: %v", expMsg, watchCtx.Err())
		}
	}
}

// makeFilesystem creates an ext4 filesystem of the requested size (in bytes) at path p.
func makeFilesystem(ctx context.Context, p string, size int64) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	// Clean up if we get an error mid-initialization.
	toClose := f
	defer func() {
		if toClose != nil {
			toClose.Close()
		}
	}()

	// Seek to the end of the requested size and write a byte.
	if _, err := f.Seek(size-1, 0); err != nil {
		return err
	}
	if _, err := f.Write([]byte{0xff}); err != nil {
		return err
	}
	toClose = nil // disarm cleanup
	if err := f.Close(); err != nil {
		return err
	}

	return testexec.CommandContext(ctx, "mkfs.ext4", p).Run(testexec.DumpLogOnError)
}

// streamDmesg clears the kernel ring buffer and then starts a dmesg process and
// asynchronously copies all log messages to a channel. The caller is responsible
// for killing and waiting on the returned process.
func streamDmesg(ctx context.Context) (*testexec.Cmd, <-chan string, error) {
	// Clear the buffer first so we don't see stale messages.
	if err := testexec.CommandContext(ctx, "dmesg", "--clear").Run(
		testexec.DumpLogOnError); err != nil {
		return nil, nil, errors.Wrap(err, "failed to clear log buffer")
	}

	// Start a dmesg process that writes messages to stdout as they're logged.
	cmd := testexec.CommandContext(ctx, "dmesg", "--follow")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start dmesg")
	}

	// Start a goroutine that just passes lines from dmesg to a channel.
	ch := make(chan string)
	go func() {
		defer close(ch)

		// Writes msg to ch and returns true if more messages should be written.
		writeMsg := func(msg string) bool {
			// To avoid blocking forever on a write to ch if nobody's reading from
			// it, we use a non-blocking write. If the channel isn't writable, sleep
			// briefly and then check if the context's deadline has been reached.
			for {
				if ctx.Err() != nil {
					return false
				}

				select {
				case ch <- msg:
					return true
				default:
					testing.Sleep(ctx, 10*time.Millisecond)
				}
			}
		}

		// The Scan method will return false once the dmesg process is killed.
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if !writeMsg(sc.Text()) {
				break
			}
		}
		// Don't bother checking sc.Err(). The test will already fail if the expected
		// message isn't seen.
	}()

	return cmd, ch, nil
}
```
