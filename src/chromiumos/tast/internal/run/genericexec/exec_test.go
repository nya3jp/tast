// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/run/genericexec"
	"chromiumos/tast/testutil"
)

func TestExecCmdRun(t *testing.T) {
	const (
		stdinData  = "input data"
		stdoutData = "output data"
		stderrData = "error data"
	)

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "executable")

	lo, err := fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		gotArgs := args[1:]
		wantArgs := []string{"foo", "bar", "baz"}
		if !cmp.Equal(gotArgs, wantArgs) {
			t.Errorf("Args mismatch: got %q, want %q", gotArgs, wantArgs)
		}

		b, err := ioutil.ReadAll(stdin)
		if err != nil {
			t.Errorf("ReadAll failed for stdin: %v", err)
		}
		if s := string(b); s != stdinData {
			t.Errorf("Stdin mismatch: got %q, want %q", s, stdinData)
		}

		if _, err := io.WriteString(stdout, stdoutData); err != nil {
			t.Errorf("Write failed for stdout: %v", err)
		}
		if _, err := io.WriteString(stderr, stderrData); err != nil {
			t.Errorf("Write failed for stderr: %v", err)
		}
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	cmd := genericexec.CommandExec(path, "foo", "bar")
	var stdout, stderr bytes.Buffer
	if err := cmd.Run(context.Background(), []string{"baz"}, bytes.NewBufferString(stdinData), &stdout, &stderr); err != nil {
		t.Errorf("Run failed: %v", err)
	}

	if s := stdout.String(); s != stdoutData {
		t.Errorf("Stdout mismatch: got %q, want %q", s, stdoutData)
	}
	if s := stderr.String(); s != stderrData {
		t.Errorf("Stderr mismatch: got %q, want %q", s, stderrData)
	}
}

func TestExecCmdRunFail(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "executable")

	lo, err := fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		return 28
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	cmd := genericexec.CommandExec(path)
	err = cmd.Run(context.Background(), []string{"baz"}, &bytes.Buffer{}, ioutil.Discard, ioutil.Discard)
	if err == nil {
		t.Error("Run unexpectedly succeeded; want exit status 28")
	} else if xerr, ok := err.(*exec.ExitError); !ok || xerr.ExitCode() != 28 {
		t.Errorf("Run failed: %v; want exit status 28", err)
	}
}

func TestExecCmdInteract(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "executable")

	lo, err := fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		gotArgs := args[1:]
		wantArgs := []string{"foo", "bar", "baz"}
		if !cmp.Equal(gotArgs, wantArgs) {
			t.Errorf("Args mismatch: got %q, want %q", gotArgs, wantArgs)
		}
		// Copy stdin to both stdout and stderr.
		io.Copy(io.MultiWriter(stdout, stderr), stdin)
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := genericexec.CommandExec(path, "foo", "bar")
	proc, err := cmd.Interact(ctx, []string{"baz"})
	if err != nil {
		t.Fatalf("Interact failed: %v", err)
	}
	defer proc.Wait(ctx)

	data := strings.Repeat("cute kittens", 10000)

	// Read stdout and stderr in separate goroutines to avoid deadlocks.
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(&stdout, proc.Stdout())
	}()
	go func() {
		defer wg.Done()
		io.Copy(&stderr, proc.Stderr())
	}()

	// Write data to stdin.
	stdin := proc.Stdin()
	if _, err := io.WriteString(stdin, data); err != nil {
		t.Errorf("Write failed for stdin: %v", err)
	}
	stdin.Close()

	// Wait for finishing to read stdout/stderr.
	wg.Wait()

	if s := stdout.String(); s != data {
		t.Errorf("Stdout mistmatch: got %q, want %q", s, data)
	}
	if s := stderr.String(); s != data {
		t.Errorf("Stderr mistmatch: got %q, want %q", s, data)
	}
}

func TestExecCmdInteractCancel(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "executable")

	lo, err := fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		// Just block until stdin is closed.
		io.Copy(ioutil.Discard, stdin)
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	interactCtx, interactCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer interactCancel()

	cmd := genericexec.CommandExec(path)
	proc, err := cmd.Interact(interactCtx, nil)
	if err != nil {
		t.Fatalf("Interact failed: %v", err)
	}

	// Cancel the context passed to Interact, which should kill the process
	// soon.
	interactCancel()

	// Wait should not take long to finish.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()

	proc.Wait(waitCtx)

	if waitCtx.Err() != nil {
		t.Fatal("Canceling the context passed to Interact did not kill the process")
	}

	// Make sure the process was killed.
	if state := proc.(*genericexec.ExecProcess).ProcessState(); state == nil {
		t.Error("Process was not killed on Wait")
	}
}

func TestExecCmdWaitCancel(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "executable")

	lo, err := fakeexec.CreateLoopback(path, func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
		// Just block until stdin is closed.
		io.Copy(ioutil.Discard, stdin)
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	interactCtx, interactCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer interactCancel()

	cmd := genericexec.CommandExec(path)
	proc, err := cmd.Interact(interactCtx, nil)
	if err != nil {
		t.Fatalf("Interact failed: %v", err)
	}

	// Passing a canceled context to Wait should close kill the process
	// soon.
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitCancel()

	proc.Wait(waitCtx)

	if interactCtx.Err() != nil {
		t.Fatal("Canceling the context passed to Wait did not kill the process")
	}

	// Make sure the process was killed.
	if state := proc.(*genericexec.ExecProcess).ProcessState(); state == nil {
		t.Error("Process was not killed on Wait")
	}
}
