// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	if err := td.Hst.Command("true").Run(td.Ctx); err != nil {
		t.Error("Failed to run true: ", err)
	}

	if err := td.Hst.Command("echo hello").Run(td.Ctx); err == nil {
		t.Error("Passing shell command worked unexpectedly")
	}
}

func TestCommandsOnCustomPlatform(t *testing.T) {
	t.Parallel()

	var expectedCmd string
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(req *sshtest.ExecReq) {
		if req.Cmd != expectedCmd {
			t.Errorf("Unexpected command %q (want %q)", req.Cmd, expectedCmd)
			req.Start(false)
			return
		}
		req.Start(true)
		req.End(0)
	})
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	platform := &ssh.Platform{
		BuildShellCommand: func(dir string, args []string) string {
			return dir + "|" + strings.Join(args, "|")
		},
	}

	ctx := context.Background()
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{ConnectRetries: 1, Platform: platform})
	if err != nil {
		t.Fatal("Unable to connect to SSH Server")
	}
	// Run a command
	cmd := hst.Command("echo", "abc")
	cmd.Dir = "/home/user/files/"
	expectedCmd = "/home/user/files/|echo|abc"
	if err := cmd.Run(ctx); err != nil {
		t.Error("Failed to run command in directory: ", err)
	}
}

func TestOutput(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello").Output(td.Ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stdout: got %q, want %q", got, want)
	}

	// Standard error is not captured.
	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello >&2").Output(td.Ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), ""; got != want {
		t.Errorf("Unexpectedly captured stderr: got %q, want %q", got, want)
	}

	// Output is available even if the command exits abnormally.
	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello; exit 1").Output(td.Ctx); err == nil {
		t.Error("No error returned for exit 1")
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Unexpected output from echo: got %q, want %q", got, want)
	}
}

func TestCombinedOutput(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello").CombinedOutput(td.Ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stdout: got %q, want %q", got, want)
	}

	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello >&2").CombinedOutput(td.Ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stderr: got %q, want %q", got, want)
	}

	// Output is available even if the command exits abnormally.
	if out, err := td.Hst.Command("/bin/sh", "-c", "echo hello; exit 1").CombinedOutput(td.Ctx); err == nil {
		t.Error("No error returned for exit 1")
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Unexpected output from echo: got %q, want %q", got, want)
	}
}

func TestStartWait(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("true")
	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.Ctx); err != nil {
		t.Fatal("Wait failed: ", err)
	}
}

func TestAbort(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("long_sleep")
	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}

	cmd.Abort()

	if err := cmd.Wait(td.Ctx); err == nil {
		t.Fatal("Wait unexpectedly succeeded")
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	checkExitCode := func(name string, err error) {
		if err == nil {
			t.Errorf("%s unexpectedly succeeded", name)
		} else if exitErr, ok := err.(*cryptossh.ExitError); !ok {
			t.Errorf("%s returned %T; want *cryptossh.ExitError", name, err)
		} else if code := exitErr.ExitStatus(); code != 28 {
			t.Errorf("%s returned exit code %d; want 28", name, code)
		}
	}

	args := []string{"/bin/sh", "-c", "exit 28"}

	err := td.Hst.Command(args[0], args[1:]...).Run(td.Ctx)
	checkExitCode("Run", err)

	_, err = td.Hst.Command(args[0], args[1:]...).Output(td.Ctx)
	checkExitCode("Output", err)

	_, err = td.Hst.Command(args[0], args[1:]...).CombinedOutput(td.Ctx)
	checkExitCode("CombinedOutput", err)

	cmd := td.Hst.Command(args[0], args[1:]...)
	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	err = cmd.Wait(td.Ctx)
	checkExitCode("Wait", err)
}

func TestDir(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	const filename = "tast_unittest.TestDir.txt"

	cmd := td.Hst.Command("touch", filename)
	cmd.Dir = dir
	if err := cmd.Run(td.Ctx); err != nil {
		t.Fatal("Run failed: ", err)
	}

	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Fatalf("%s does not exist", filename)
	}
}

func TestStdin(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	const want = "hello"

	cmd := td.Hst.Command("cat")
	cmd.Stdin = bytes.NewBufferString(want)
	if out, err := cmd.Output(td.Ctx); err != nil {
		t.Fatal("Output failed: ", err)
	} else if got := string(out); got != want {
		t.Fatalf("Output returned %q; want %q", got, want)
	}
}

func TestStdoutStderr(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	var stdout, stderr bytes.Buffer

	cmd := td.Hst.Command("/bin/sh", "-c", "echo hello; echo world >&2")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(td.Ctx); err != nil {
		t.Fatal("Run failed: ", err)
	}

	if got, want := stdout.String(), "hello\n"; got != want {
		t.Errorf("Stdout got %q; want %q", got, want)
	}
	if got, want := stderr.String(), "world\n"; got != want {
		t.Errorf("Stderr got %q; want %q", got, want)
	}
}

func TestStdinPipe(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	const want = "hello"

	cmd := td.Hst.Command("cat")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("StdinPipe failed: ", err)
	}

	go func() {
		io.WriteString(stdin, want)
		stdin.Close()
	}()

	if out, err := cmd.Output(td.Ctx); err != nil {
		t.Fatal("Output failed: ", err)
	} else if got := string(out); got != want {
		t.Fatalf("Output returned %q; want %q", got, want)
	}
}

func TestStdoutPipeStderrPipe(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("/bin/sh", "-c", "echo hello; echo world >&2")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		if b, err := ioutil.ReadAll(stdout); err != nil {
			t.Error("Failed to read stdout: ", err)
		} else if got, want := string(b), "hello\n"; got != want {
			t.Errorf("Stdout got %q; want %q", got, want)
		}
	}()

	go func() {
		defer wg.Done()

		if b, err := ioutil.ReadAll(stderr); err != nil {
			t.Error("Failed to read stderr: ", err)
		} else if got, want := string(b), "world\n"; got != want {
			t.Errorf("Stderr got %q; want %q", got, want)
		}
	}()

	wg.Wait()

	if err := cmd.Wait(td.Ctx); err != nil {
		t.Error("Wait failed: ", err)
	}
}

func TestPipesClosedOnWait(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("true")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.Ctx); err != nil {
		t.Fatal("Wait failed: ", err)
	}

	ch := make(chan struct{})
	go func() {
		// These I/O operations should not block.
		ioutil.ReadAll(stdout)
		ioutil.ReadAll(stderr)
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("I/O operations blocked after Wait")
	}
}

func TestPipesClosedOnAbort(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("long_sleep")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}

	cmd.Abort()

	ch := make(chan struct{})
	go func() {
		// These I/O operations should not block.
		ioutil.ReadAll(stdout)
		ioutil.ReadAll(stderr)
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("I/O operations blocked after Abort")
	}

	if err := cmd.Wait(td.Ctx); err == nil {
		t.Fatal("Wait unexpectedly succeeded")
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.ExecTimeout = sshtest.EndTimeout

	if err := td.Hst.Command("true").Run(td.Ctx); err == nil {
		t.Fatal("Run did not honor the timeout")
	}
}

func TestOutputTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.ExecTimeout = sshtest.EndTimeout

	if _, err := td.Hst.Command("true").Output(td.Ctx); err == nil {
		t.Fatal("Output did not honor the timeout")
	}
}

func TestCombinedOutputTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.ExecTimeout = sshtest.EndTimeout

	if _, err := td.Hst.Command("true").CombinedOutput(td.Ctx); err == nil {
		t.Fatal("CombinedOutput did not honor the timeout")
	}
}

func TestStartTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.ExecTimeout = sshtest.StartTimeout

	cmd := td.Hst.Command("true")
	if err := cmd.Start(td.Ctx); err == nil {
		defer cmd.Wait(td.Ctx)
		t.Fatal("Start did not honor the timeout")
	}
}

func TestWaitTimeout(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.ExecTimeout = sshtest.EndTimeout

	cmd := td.Hst.Command("true")
	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.Ctx); err == nil {
		t.Fatal("Wait did not honor the timeout")
	}
}

func TestWaitTwice(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	cmd := td.Hst.Command("true")
	if err := cmd.Start(td.Ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.Ctx); err != nil {
		t.Fatal("First Wait failed: ", err)
	}
	// Second Wait call fails, but it should not panic.
	if err := cmd.Wait(td.Ctx); err == nil {
		t.Fatal("Second Wait succeeded")
	}
}

func TestDumpLogOnError(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	type cmd func(context.Context, ...ssh.RunOption) error
	type cmd2 func(context.Context, ...ssh.RunOption) ([]byte, error)

	for i, tc := range []struct {
		f          func(c *ssh.Cmd) cmd
		f2         func(c *ssh.Cmd) cmd2
		fail       bool
		wantStdout bool
		wantStderr bool
	}{{
		f:          func(c *ssh.Cmd) cmd { return c.Run },
		fail:       true,
		wantStdout: true,
		wantStderr: true,
	}, {
		f:          func(c *ssh.Cmd) cmd { return c.Run },
		fail:       false,
		wantStdout: false,
		wantStderr: false,
	}, {
		f2:         func(c *ssh.Cmd) cmd2 { return c.Output },
		fail:       true,
		wantStdout: false,
		wantStderr: true,
	}, {
		f2:         func(c *ssh.Cmd) cmd2 { return c.CombinedOutput },
		fail:       true,
		wantStdout: false,
		wantStderr: false,
	}} {
		t.Logf("Test#%d:", i)

		// `echo "f"oo` doesn't match foo in itself, but produces `foo` when
		// invoked.
		script := `echo "f"oo; echo "b"ar >&2`
		if tc.fail {
			script += `; false`
		}
		cmd := td.Hst.Command("sh", "-c", script)

		var log bytes.Buffer

		ctx := testcontext.WithLogger(context.Background(), func(msg string) {
			fmt.Fprint(&log, msg)
		})
		var err error
		if tc.f != nil {
			err = tc.f(cmd)(ctx, ssh.DumpLogOnError)
		} else {
			_, err = tc.f2(cmd)(ctx, ssh.DumpLogOnError)
		}

		if !tc.fail && err != nil {
			t.Fatal("Got error: ", err)
		} else if tc.fail && err == nil {
			t.Fatal("Got no error")
		}

		if got, want := strings.Contains(log.String(), "foo"), tc.wantStdout; got != want {
			if got {
				t.Errorf("Log %q contains %q", log.String(), "foo")
			} else {
				t.Errorf("Log %q does not contain %q", log.String(), "foo")
			}
		}
		if got, want := strings.Contains(log.String(), "bar"), tc.wantStderr; got != want {
			if got {
				t.Errorf("Log %q contains %q", log.String(), "bar")
			} else {
				t.Errorf("Log %q does not contain %q", log.String(), "bar")
			}
		}
	}
}

func TestSameStdoutAndStderr(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	longx := "x"
	longy := "yy"
	for i := 0; i < 7; i++ { // repeat the original 128 times
		longx += longx
		longy += longy
	}

	const n = 50
	script := fmt.Sprintf(`sh -c 'for _ in $(seq 1 %d); do echo "%s" &
echo "%s" >&2 &
done &'`, n, longx, longy)
	cmd := td.Hst.Command("sh", "-c", script)

	var w bytes.Buffer
	cmd.Stderr = &w
	cmd.Stdout = &w

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	x := 0
	y := 0
	for _, s := range strings.Split(strings.TrimSpace(w.String()), "\n") {
		switch s {
		case longx:
			x++
		case longy:
			y++
		default:
			t.Errorf("Got unexpected line %q", s)
		}
	}
	if x != n {
		t.Errorf("Got x = %d, want %d", x, n)
	}
	if y != n {
		t.Errorf("Got y = %d, want %d", y, n)
	}
}
