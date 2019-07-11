// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"chromiumos/tast/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	if err := td.hst.Command("true").Run(td.ctx); err != nil {
		t.Error("Failed to run true: ", err)
	}

	if err := td.hst.Command("echo hello").Run(td.ctx); err == nil {
		t.Error("Passing shell command worked unexpectedly")
	}
}

func TestOutput(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello").Output(td.ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stdout: got %q, want %q", got, want)
	}

	// Standard error is not captured.
	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello >&2").Output(td.ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), ""; got != want {
		t.Errorf("Unexpectedly captured stderr: got %q, want %q", got, want)
	}

	// Output is available even if the command exits abnormally.
	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello; exit 1").Output(td.ctx); err == nil {
		t.Error("No error returned for exit 1")
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Unexpected output from echo: got %q, want %q", got, want)
	}
}

func TestCombinedOutput(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello").CombinedOutput(td.ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stdout: got %q, want %q", got, want)
	}

	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello >&2").CombinedOutput(td.ctx); err != nil {
		t.Error("Failed to run echo: ", err)
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Failed to capture stderr: got %q, want %q", got, want)
	}

	// Output is available even if the command exits abnormally.
	if out, err := td.hst.Command("/bin/sh", "-c", "echo hello; exit 1").CombinedOutput(td.ctx); err == nil {
		t.Error("No error returned for exit 1")
	} else if got, want := string(out), "hello\n"; got != want {
		t.Errorf("Unexpected output from echo: got %q, want %q", got, want)
	}
}

func TestStartWait(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("true")
	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.ctx); err != nil {
		t.Fatal("Wait failed: ", err)
	}
}

func TestAbort(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("long_sleep")
	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}

	cmd.Abort()

	if err := cmd.Wait(td.ctx); err == nil {
		t.Fatal("Wait unexpectedly succeeded")
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	checkExitCode := func(name string, err error) {
		if err == nil {
			t.Errorf("%s unexpectedly succeeded", name)
		} else if exitErr, ok := err.(*ssh.ExitError); !ok {
			t.Errorf("%s returned %T; want *ssh.ExitError", name, err)
		} else if code := exitErr.ExitStatus(); code != 28 {
			t.Errorf("%s returned exit code %d; want 28", name, code)
		}
	}

	args := []string{"/bin/sh", "-c", "exit 28"}

	err := td.hst.Command(args[0], args[1:]...).Run(td.ctx)
	checkExitCode("Run", err)

	_, err = td.hst.Command(args[0], args[1:]...).Output(td.ctx)
	checkExitCode("Output", err)

	_, err = td.hst.Command(args[0], args[1:]...).CombinedOutput(td.ctx)
	checkExitCode("CombinedOutput", err)

	cmd := td.hst.Command(args[0], args[1:]...)
	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	err = cmd.Wait(td.ctx)
	checkExitCode("Wait", err)
}

func TestDir(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	const filename = "tast_unittest.TestDir.txt"

	cmd := td.hst.Command("touch", filename)
	cmd.Dir = dir
	if err := cmd.Run(td.ctx); err != nil {
		t.Fatal("Run failed: ", err)
	}

	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Fatalf("%s does not exist", filename)
	}
}

func TestStdin(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	const want = "hello"

	cmd := td.hst.Command("cat")
	cmd.Stdin = bytes.NewBufferString(want)
	if out, err := cmd.Output(td.ctx); err != nil {
		t.Fatal("Output failed: ", err)
	} else if got := string(out); got != want {
		t.Fatalf("Output returned %q; want %q", got, want)
	}
}

func TestStdoutStderr(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	var stdout, stderr bytes.Buffer

	cmd := td.hst.Command("/bin/sh", "-c", "echo hello; echo world >&2")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(td.ctx); err != nil {
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
	td := newTestData(t)
	defer td.close()

	const want = "hello"

	cmd := td.hst.Command("cat")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("StdinPipe failed: ", err)
	}

	go func() {
		io.WriteString(stdin, want)
		stdin.Close()
	}()

	if out, err := cmd.Output(td.ctx); err != nil {
		t.Fatal("Output failed: ", err)
	} else if got := string(out); got != want {
		t.Fatalf("Output returned %q; want %q", got, want)
	}
}

func TestStdoutPipeStderrPipe(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("/bin/sh", "-c", "echo hello; echo world >&2")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.ctx); err != nil {
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

	if err := cmd.Wait(td.ctx); err != nil {
		t.Error("Wait failed: ", err)
	}
}

func TestPipesClosedOnWait(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("true")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.ctx); err != nil {
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
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("long_sleep")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("StdoutPipe failed: ", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("StderrPipe failed: ", err)
	}

	if err := cmd.Start(td.ctx); err != nil {
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

	if err := cmd.Wait(td.ctx); err == nil {
		t.Fatal("Wait unexpectedly succeeded")
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.execTimeout = endTimeout

	if err := td.hst.Command("true").Run(td.ctx); err == nil {
		t.Fatal("Run did not honor the timeout")
	}
}

func TestOutputTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.execTimeout = endTimeout

	if _, err := td.hst.Command("true").Output(td.ctx); err == nil {
		t.Fatal("Output did not honor the timeout")
	}
}

func TestCombinedOutputTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.execTimeout = endTimeout

	if _, err := td.hst.Command("true").CombinedOutput(td.ctx); err == nil {
		t.Fatal("CombinedOutput did not honor the timeout")
	}
}

func TestStartTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.execTimeout = startTimeout

	cmd := td.hst.Command("true")
	if err := cmd.Start(td.ctx); err == nil {
		defer cmd.Wait(td.ctx)
		t.Fatal("Start did not honor the timeout")
	}
}

func TestWaitTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.execTimeout = endTimeout

	cmd := td.hst.Command("true")
	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.ctx); err == nil {
		t.Fatal("Wait did not honor the timeout")
	}
}

func TestWaitTwice(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	cmd := td.hst.Command("true")
	if err := cmd.Start(td.ctx); err != nil {
		t.Fatal("Start failed: ", err)
	}
	if err := cmd.Wait(td.ctx); err != nil {
		t.Fatal("First Wait failed: ", err)
	}
	// Second Wait call fails, but it should not panic.
	if err := cmd.Wait(td.ctx); err == nil {
		t.Fatal("Second Wait succeeded")
	}
}
