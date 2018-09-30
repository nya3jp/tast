# Tast Quickstart (go/tast-quickstart)

[TOC]

## Prerequisites

You'll need a [Chrome OS chroot]. If you've only done Chrome development so far,
note that this is different from the Chrome checkout described in the [Simple
Chrome] documentation.

You'll also need a Chrome OS device running a system image built with the `test`
flag that's reachable from your workstation via SSH. An image running in a
[virtual machine] will also work. If you're using a test image that you
downloaded rather than one built in your chroot, make sure that it's a recent
version.

## Run a prebuilt test

In your chroot, run the following:

```sh
tast -verbose run -build=false <test-device-ip> ui.ChromeLogin
```

You should see output scroll by on your workstation, and on the Chrome OS
device, the test should log in and load a webpage. After the test is done, take
a look at the results in `/tmp/tast/results/latest` in your chroot.

## Build and run a test

The previous step ran a test that was already built into your device's system
image, but you can also use the `tast` command to quickly rebuild all tests and
push them to the device.

In your chroot, run the same command as before **but without the `-build=false`
argument**:

```sh
tast -verbose run <test-device-ip> ui.ChromeLogin
```

This time, the command will take a bit longer (but build objects will be
cached). The test should succeed again.

> The first time you run this, or after you sync your checkout, you may see an
> error similar to the following:
```
To install missing dependencies, run:

 sudo emerge -j 16 \
   =dev-go/cdp-0.9.1-r1 \
   =dev-go/dbus-0.0.2-r5
```
> This is expected: to speed things up, `tast` is building the tests directly
> instead of emerging the `tast-local-tests-cros` package, so it needs some help
> from you to make sure that all required dependencies are installed. If you run
> the provided `emerge` command, the `tast` command should work when re-run.

See [Running Tests] for more information.

## Modify a test

Now, let's modify the test. In your Chrome OS checkout, go to
`src/platform/tast-tests/src/chromiumos/tast/local/bundles/cros/ui` and open
`chrome_login.go` (for convenience, there's also a `local_tests` symlink at the
top of `tast-tests`). The `ChromeLogin` function here will run directly on the
test device.

At the end of the function, add the following code:

```go
if _, err = cr.NewConn(ctx, "https://www.google.com/"); err != nil {
	s.Error("Failed to open page: ", err)
}
```

Back in your chroot, run `tast` again:

```sh
tast -verbose run <test-device-ip> ui.ChromeLogin
```

This time, the test should additionally open a Google search page.

Return to the test file and add the following statement at the end of the
function:

```go
s.Error("This is an intentional error")
```

If you build and run the test again, you should see it fail.

See [Writing Tests] for more information, and explore the [tast-tests
repository] to see existing tests and related packages.

## Next steps

Additional Tast documentation is available in the [tast repository].

Many resources are available for learning more about Go. Here are a few:

*   [A Tour of Go] - In-browser introduction to Go's core features.
*   [Official Go documentation] - Package documentation, specifications, blog
    posts, and recorded talks.
*   [Community Learn wiki] - Links to external resources.

[Chrome OS chroot]: http://www.chromium.org/chromium-os/quick-start-guide
[Simple Chrome]: https://chromium.googlesource.com/chromiumos/docs/+/master/simple_chrome_workflow.md
[virtual machine]: https://chromium.googlesource.com/chromiumos/docs/+/master/cros_vm.md
[Running Tests]: running_tests.md
[Writing Tests]: writing_tests.md
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[tast repository]: https://chromium.googlesource.com/chromiumos/platform/tast/
[A Tour of Go]: https://tour.golang.org/
[Official Go documentation]: https://golang.org/doc/
[Community Learn wiki]: https://github.com/golang/go/wiki/Learn
