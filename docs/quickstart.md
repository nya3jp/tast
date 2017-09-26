# Tast Quickstart

[TOC]

## Prerequisites

You'll need a [Chrome OS chroot].

You'll also need a Chrome OS device running a system image built with the `test`
flag that's reachable from your workstation via SSH. An image running in a
[virtual machine] will also work.

## Run a test

In your chroot, run the following:

```sh
tast -verbose run -build=false <test-device-ip> ui.ChromeSanity
```

You should see output scroll by on your workstation, and on the Chrome OS
device, the test should log in and load a webpage. After the test is done, take
a look at the results in `/tmp/tast/results/latest` in your chroot.

See [Running Tests] for more information.

## Modify a test

In your Chrome OS checkout, go to
`src/platform/tast-tests/src/chromiumos/tast/local/tests/ui` and open
`chrome_sanity.go`. The `ChromeSanity` function here will run directly on the
test device.

At the end of the function, add the following code:

```go
if _, err = cr.NewConn(s.Context(), "https://www.google.com/"); err != nil {
	s.Error("Failed to open page: ", err)
}
```

Back in your chroot, run the following:

```sh
tast -verbose run <test-device-ip> ui.ChromeSanity
```

(Note how `-build=false` is omitted; as a result, the `tast` command will
rebuild tests locally and push them to the device. You may be prompted to
install some dependencies needed to build tests; if so, run the provided
`emerge` command and then re-run the `tast` command.)

This time, the updated test should additionally open a Google search page.

Return to the test file and add the following statement at the end of the
function:

```go
s.Error("This is an intentional error")
```

If you build and run the test again, you should see it fail.

See [Writing Tests] for more information.

Many resources are available for learning more about Go. Here are a few:

*   [A Tour of Go] - In-browser introduction to Go's core features.
*   [Official Go documentation] - Package documentation, specifications, blog
    posts, and recorded talks.
*   [Community Learn wiki] - Links to external resources.

[virtual machine]: https://www.chromium.org/chromium-os/how-tos-and-troubleshooting/running-chromeos-image-under-virtual-machines
[Chrome OS chroot]: http://www.chromium.org/chromium-os/quick-start-guide
[Running Tests]: running_tests.md
[Writing Tests]: writing_tests.md
[A Tour of Go]: https://tour.golang.org/
[Official Go documentation]: https://golang.org/doc/
[Community Learn wiki]: https://github.com/golang/go/wiki/Learn
