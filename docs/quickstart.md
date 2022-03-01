# Tast Quickstart (go/tast-quickstart)

[TOC]

## Prerequisites

You'll need a [Chrome OS chroot]. If you've only done Chrome development so far,
note that this is different from the Chrome checkout described in the
[Simple Chrome] documentation.

You'll also need a Chrome OS device running a system image built with the `test`
flag that's reachable from your workstation via SSH. An image running in a
[virtual machine] will also work. If you're using a test image that you
downloaded rather than one built in your chroot, make sure that it's a recent
version.

<a name="dataloss"></a> **WARNING: Potential data loss:**  Many Tast tests
remove all user profiles from the device when run, including any local state.
Prefer to have a device specifically for testing.

[Chrome OS chroot]: http://www.chromium.org/chromium-os/quick-start-guide
[Simple Chrome]: https://chromium.googlesource.com/chromiumos/docs/+/main/simple_chrome_workflow.md
[virtual machine]: https://chromium.googlesource.com/chromiumos/docs/+/main/cros_vm.md

## Setup

### Update Chrome OS chroot

Assuming that you already have a valid Chrome OS repo checked out (see
[Chrome OS chroot]), it is recommended to update the chroot by doing:

```sh
cd ${CHROMEOS_SRC}
chromite/bin/cros_sdk    # to enter chroot
./update_chroot          # makes sure that the latest dependencies are installed
```

### IDE

Any [modern editor] supports Go. The following are the instructions to setup
[Visual Studio Code] with Tast code navigation:

1.  Download [Visual Studio Code]
2.  Install the [official Go extension] (VSCode will recommend that extension
    once you open a Go file)
3.  Update the `GOPATH` environment variable to make code navigation works (`F12` key)

    ```sh
    mkdir ~/go
    # Main GOPATH, where extra binaries will get installed.
    export GOPATH=$HOME/go
    # Append Tast repos to GOPATH
    export GOPATH=${GOPATH}:${CHROMEOS_SRC}/src/platform/tast-tests:${CHROMEOS_SRC}/src/platform/tast
    # Append Tast dependencies
    export GOPATH=${GOPATH}:${CHROMEOS_SRC}/chroot/usr/lib/gopath
    ```

4.  Start Visual Studio Code with Tast

    ```sh
    cd ${CHROMEOS_SRC}/src/platform/
    code ./tast-tests ./tast
    ```

Note: If you are using the VSCode "Remote-SSH" extension, restart
VSCode's SSH server after setting the GOPATH, otherwise the Go
extension won't pick it up. For example, using the VSCode command
palette, you can run `>Remote-SSH: Kill VS Code Server on Host`.

After that, it's useful to add the following to your settings JSON to
avoid opening a 404 page whenever you try to follow links from import
statements:

```
  "gopls": {
    "ui.navigation.importShortcut": "Definition"
  },
```

https://github.com/golang/vscode-go/issues/237#issuecomment-646067281

[modern editor]: https://github.com/golang/go/wiki/IDEsAndTextEditorPlugins
[Visual Studio Code]: https://code.visualstudio.com/
[official Go extension]: https://code.visualstudio.com/docs/languages/go

## Run a prebuilt test

**WARNING: Potential data loss:** Tast [may delete](#dataloss) profiles and
local state.

In your chroot, run the following:

```sh
tast -verbose run -build=false <test-device-ip> login.Chrome
```

You should see output scroll by on your workstation, and on the Chrome OS
device, the test should log in and load a webpage. After the test is done, take
a look at the results in `/tmp/tast/results/latest` in your chroot.

## Build and run a test

**WARNING: Potential data loss:** Tast [may delete](#dataloss) profiles and
local state.

The previous step ran a test that was already built into your device's system
image, but you can also use the `tast` command to quickly rebuild all tests and
push them to the device.

In your chroot, run the same command as before **but without the `-build=false`
argument**:

```sh
tast -verbose run <test-device-ip> login.Chrome
```

This time, the command will take a bit longer (but build objects will be
cached). The test should succeed again.

See [Running Tests] for more information.

[Running Tests]: running_tests.md

## Modify a test

Now, let's modify the test. In your Chrome OS checkout, go to
`src/platform/tast-tests/src/chromiumos/tast/local/bundles/cros/login` and open
`chrome.go` (for convenience, there's also a `local_tests` symlink at the
top of `tast-tests`). The `Chrome` function here will run directly on the
test device.

At the end of the anonymous function inside `testChromeLogin`, add the following code:

```go
if _, err = cr.NewConn(ctx, "https://www.google.com/"); err != nil {
	s.Error("Failed to open page: ", err)
}
```

Back in your chroot, run `tast` again:

```sh
tast -verbose run <test-device-ip> login.Chrome
```

This time, the test should additionally open a Google search page.

Return to the test file and add the following statement at the end of the
anonymous function inside `testChromeLogin`:

```go
s.Error("This is an intentional error")
```

If you build and run the test again, you should see it fail.

## Next steps

See [Writing Tests] for more information, and explore the
[tast-tests repository] to see existing tests and related packages. [Codelab #1]
walks through the creation of a simple test.

Additional Tast documentation is available in the [tast repository].

Many resources are available for learning more about Go. Here are a few:

*   [A Tour of Go] - In-browser introduction to Go's core features.
*   [Official Go documentation] - Package documentation, specifications, blog
    posts, and recorded talks.
*   [Go at Google: Language Design in the Service of Software Engineering] -
    High-level overview of Go's features and design philosophy.
*   [Community Learn wiki] - Links to external resources.

[Writing Tests]: writing_tests.md
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/
[Codelab #1]: codelab_1.md
[tast repository]: https://chromium.googlesource.com/chromiumos/platform/tast/
[A Tour of Go]: https://tour.golang.org/
[Official Go documentation]: https://golang.org/doc/
[Go at Google: Language Design in the Service of Software Engineering]: https://talks.golang.org/2012/splash.article
[Community Learn wiki]: https://github.com/golang/go/wiki/Learn
