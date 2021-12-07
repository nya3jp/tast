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
	"strings"
	"sync"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	"chromiumos/tast/internal/run/genericexec"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testutil"
)

func TestSSHCmdRun(t *testing.T) {
	const (
		stdinData  = "input data"
		stdoutData = "output data"
		stderrData = "error data"
	)

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		const wantCmd = "exec executable foo bar baz"
		if req.Cmd != wantCmd {
			t.Errorf("Command mismatch: got %q, want %q", req.Cmd, wantCmd)
		}

		req.Start(true)

		b, err := ioutil.ReadAll(req)
		if err != nil {
			t.Errorf("ReadAll failed for stdin: %v", err)
		}
		if s := string(b); s != stdinData {
			t.Errorf("Stdin mismatch: got %q, want %q", s, stdinData)
		}

		if _, err := io.WriteString(req, stdoutData); err != nil {
			t.Errorf("Write failed for stdout: %v", err)
		}
		if _, err := io.WriteString(req.Stderr(), stderrData); err != nil {
			t.Errorf("Write failed for stderr: %v", err)
		}

		req.End(0)
	})
	defer td.Close()

	ctx := context.Background()

	conn, err := ssh.New(context.Background(), &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	cmd := genericexec.CommandSSH(conn, "executable", "foo", "bar")
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

func TestSSHCmdRunFail(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		req.Start(true)
		req.End(28)
	})
	defer td.Close()

	ctx := context.Background()

	conn, err := ssh.New(context.Background(), &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	cmd := genericexec.CommandSSH(conn, "executable")
	err = cmd.Run(context.Background(), []string{"baz"}, &bytes.Buffer{}, ioutil.Discard, ioutil.Discard)
	if err == nil {
		t.Error("Run unexpectedly succeeded; want exit status 28")
	} else if xerr, ok := err.(*cryptossh.ExitError); !ok || xerr.ExitStatus() != 28 {
		t.Errorf("Run failed: %v; want exit status 28", err)
	}
}

func TestSSHCmdInteract(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		const wantCmd = "exec executable foo bar baz"
		if req.Cmd != wantCmd {
			t.Errorf("Command mismatch: got %q, want %q", req.Cmd, wantCmd)
		}

		// Copy stdin to both stdout and stderr.
		req.Start(true)
		io.Copy(io.MultiWriter(req, req.Stderr()), req)
		req.End(0)
	})
	defer td.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := ssh.New(context.Background(), &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	cmd := genericexec.CommandSSH(conn, "executable", "foo", "bar")
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

func TestSSHCmdInteractCancel(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	started := make(chan struct{})
	finished := make(chan struct{})
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		// Just block until stdin is closed.
		close(started)
		req.Start(true)
		io.Copy(ioutil.Discard, req)
		req.End(0)
		close(finished)
	})
	defer td.Close()

	interactCtx, interactCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer interactCancel()

	conn, err := ssh.New(context.Background(), &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(interactCtx)

	cmd := genericexec.CommandSSH(conn, "executable", "foo", "bar")
	proc, err := cmd.Interact(interactCtx, []string{"baz"})
	if err != nil {
		t.Fatalf("Interact failed: %v", err)
	}

	// Wait for the callback to be called.
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("Remote command was not called")
	}

	// Cancel the context passed to Interact. This should close stdin and
	// make the process finish.
	interactCancel()

	// Wait should not take long to finish.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()

	proc.Wait(waitCtx)

	if waitCtx.Err() != nil {
		t.Error("Canceling the context passed to Interact did not abort the process")
	}

	// Stdin of the process should have been closed.
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Error("Stdin of the process was not closed")
	}
}

func TestSSHCmdWaitCancel(t *testing.T) {
	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	started := make(chan struct{})
	finished := make(chan struct{})
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		// Just block until stdin is closed.
		close(started)
		req.Start(true)
		io.Copy(ioutil.Discard, req)
		req.End(0)
		close(finished)
	})
	defer td.Close()

	interactCtx, interactCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer interactCancel()

	conn, err := ssh.New(context.Background(), &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(interactCtx)

	cmd := genericexec.CommandSSH(conn, "executable", "foo", "bar")
	proc, err := cmd.Interact(interactCtx, []string{"baz"})
	if err != nil {
		t.Fatalf("Interact failed: %v", err)
	}

	// Wait for the callback to be called.
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("Remote command was not called")
	}

	// Passing a canceled context to Wait should close stdin and make
	// the process finish.
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitCancel()

	proc.Wait(waitCtx)

	if interactCtx.Err() != nil {
		t.Error("Canceling the context passed to Wait did not abort the process")
	}

	// Stdin of the process should have been closed.
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Error("Stdin of the process was not closed")
	}
}
