// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakeexec_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"

	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/rpc"
)

func mustCreateLoopback(t *testing.T, proc fakeexec.ProcFunc) (lo *fakeexec.Loopback, path string) {
	t.Helper()

	f, err := ioutil.TempFile("", "tast-fakeexec.")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	path = f.Name()

	lo, err = fakeexec.CreateLoopback(path, proc)
	if err != nil {
		t.Fatal(err)
	}
	return lo, path
}

func TestLoopbackArgs(t *testing.T) {
	want := []string{"foo", "bar"}

	lo, path := mustCreateLoopback(t, func(args []string, _ io.Reader, _, _ io.WriteCloser) int {
		if diff := cmp.Diff(args[1:], want); diff != "" {
			t.Errorf("Unexpected arguments (-got +want):\n%s", diff)
		}
		return 0
	})
	defer lo.Close()

	if err := exec.Command(path, want...).Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestLoopbackExitCode(t *testing.T) {
	lo, path := mustCreateLoopback(t, func(_ []string, _ io.Reader, _, _ io.WriteCloser) int {
		return 28
	})
	defer lo.Close()

	err := exec.Command(path).Run()
	if xerr, ok := err.(*exec.ExitError); !ok || xerr.ProcessState.ExitCode() != 28 {
		t.Errorf("Unexpected exit status: got %v; want 28", err)
	}
}

func TestLoopbackStdin(t *testing.T) {
	want := bytes.Repeat([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, 1024)

	lo, path := mustCreateLoopback(t, func(_ []string, stdin io.Reader, _, _ io.WriteCloser) int {
		got, err := ioutil.ReadAll(stdin)
		if err != nil {
			t.Errorf("Reading stdin: %v", err)
			return 1
		}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Stdin data mismatch (-got +want):\n%s", diff)
		}
		return 0
	})
	defer lo.Close()

	cmd := exec.Command(path)
	cmd.Stdin = bytes.NewBuffer(want)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestLoopbackStdout(t *testing.T) {
	want := bytes.Repeat([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, 1024)

	lo, path := mustCreateLoopback(t, func(_ []string, _ io.Reader, stdout, _ io.WriteCloser) int {
		stdout.Write(want)
		return 0
	})
	defer lo.Close()

	got, err := exec.Command(path).Output()
	if err != nil {
		t.Fatalf("Output failed: %v", err)
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Stdout data mismatch (-got +want):\n%s", diff)
	}
}

func TestLoopbackStderr(t *testing.T) {
	want := bytes.Repeat([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, 1024)

	lo, path := mustCreateLoopback(t, func(_ []string, _ io.Reader, _, stderr io.WriteCloser) int {
		stderr.Write(want)
		return 0
	})
	defer lo.Close()

	cmd := exec.Command(path)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got := buf.Bytes()
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Stderr data mismatch (-got +want):\n%s", diff)
	}
}

func TestLoopbackStdoutStderr(t *testing.T) {
	want := bytes.Repeat([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, 1024)

	lo, path := mustCreateLoopback(t, func(_ []string, _ io.Reader, stdout, stderr io.WriteCloser) int {
		// Write to stdout and stderr in an interleaved way.
		for i := 0; i < len(want); i++ {
			stdout.Write(want[i : i+1])
			stderr.Write(want[i : i+1])
		}
		return 0
	})
	defer lo.Close()

	cmd := exec.Command(path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if got := stdout.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("Stdout data mismatch (-got +want):\n%s", cmp.Diff(got, want))
	}
	if got := stderr.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("Stderr data mismatch (-got +want):\n%s", cmp.Diff(got, want))
	}
}

// TestLoopbackGRPC starts a gRPC server on a loopback executable and makes sure
// we can call its methods successfully.
// This is essentially gRPC on gRPC since loopback executables are implemented
// on top of gRPC.
func TestLoopbackGRPC(t *testing.T) {
	// Create a fake executable that serves gRPC services on pipes.
	lo, path := mustCreateLoopback(t, func(_ []string, stdin io.Reader, stdout, _ io.WriteCloser) int {
		srv := grpc.NewServer()
		reflection.Register(srv)
		srv.Serve(rpc.NewPipeListener(stdin, stdout))
		return 0
	})
	defer lo.Close()

	// Start a subprocess and connect to the gRPC server via pipes.
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	conn, err := rpc.NewPipeClientConn(context.Background(), stdout, stdin)
	if err != nil {
		t.Fatalf("Failed to connect to gRPC server via pipes: %v", err)
	}
	defer conn.Close()

	// Try calling some methods.
	cl := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
	st, err := cl.ServerReflectionInfo(context.Background())
	if err != nil {
		t.Fatal("ServerReflectionInfo RPC call failed: ", err)
	}
	if err := st.CloseSend(); err != nil {
		t.Fatal("ServerReflectionInfo RPC call failed on CloseSend: ", err)
	}
	if _, err := st.Recv(); err == nil {
		t.Fatal("ServerReflectionInfo RPC call returned an unexpected reply")
	} else if err != io.EOF {
		t.Fatal("ServerReflectionInfo RPC call failed on Recv: ", err)
	}
}
