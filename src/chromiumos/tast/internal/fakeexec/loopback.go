// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakeexec

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/shutil"
)

// ProcFunc is a callback passed to CreateLoopback to fully control the behavior of
// a loopback process.
type ProcFunc func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int

// Exec implements LoopbackExecService.
func (p ProcFunc) Exec(srv protocol.LoopbackExecService_ExecServer) error {
	// Receive ExecInit.
	req, err := srv.Recv()
	if err != nil {
		return err
	}
	init := req.GetType().(*protocol.ExecRequest_Init).Init

	stdin := &execIn{srv: srv}
	stdout := &execOut{
		srv: srv,
		ctor: func(ev *protocol.PipeEvent) *protocol.ExecResponse {
			return &protocol.ExecResponse{Type: &protocol.ExecResponse_Stdout{Stdout: ev}}
		}}
	stderr := &execOut{
		srv: srv,
		ctor: func(ev *protocol.PipeEvent) *protocol.ExecResponse {
			return &protocol.ExecResponse{Type: &protocol.ExecResponse_Stderr{Stderr: ev}}
		}}

	// Run the callback.
	code := p(init.GetArgs(), stdin, stdout, stderr)

	// Send ExitEvent.
	return srv.Send(&protocol.ExecResponse{Type: &protocol.ExecResponse_Exit{Exit: &protocol.ExitEvent{Code: int32(code)}}})
}

// Loopback represents a loopback executable file.
type Loopback struct {
	srv  *grpc.Server
	path string
}

// CreateLoopback creates a new file called a loopback executable.
//
// When a loopback executable file is executed, its process connects to the
// current unit test process by gRPC to call proc remotely. The process behaves
// exactly as specified by proc. Since proc is called within the current unit
// test process, unit tests and subprocesses can interact easily with Go
// constructs, e.g. shared memory or channels.
//
// A drawback is that proc can only emulate args, stdio and exit code. If you
// need to do anything else, e.g. catching signals, use NewAuxMain instead.
//
// Once you're done with a loopback executable, call Loopback.Close to release
// associated resources.
func CreateLoopback(path string, proc ProcFunc) (lo *Loopback, retErr error) {
	// Listen on a local port.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	port := lis.Addr().(*net.TCPAddr).Port
	defer func() {
		if retErr != nil {
			lis.Close()
		}
	}()

	// Create a loopback executable file.
	script, err := buildScript(port)
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(path, script, 0755); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			os.Remove(path)
		}
	}()

	// Make sure the file has executable bit.
	if err := os.Chmod(path, 0755); err != nil {
		return nil, err
	}

	// Finally start a gRPC server.
	srv := grpc.NewServer()
	protocol.RegisterLoopbackExecServiceServer(srv, proc)
	go srv.Serve(lis)
	return &Loopback{srv: srv, path: path}, nil
}

func buildScript(port int) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	script := fmt.Sprintf(`#!/bin/sh
%s=%d exec %s "$@"
`, portEnvName, port, shutil.Escape(exe))
	return []byte(script), nil
}

// Close removes the loopback executable file and releases its associated
// resources.
func (s *Loopback) Close() error {
	s.srv.GracefulStop()
	return os.Remove(s.path)
}

// execIn implements io.Reader which reads from the loopback process stdin.
type execIn struct {
	srv    protocol.LoopbackExecService_ExecServer
	buf    []byte
	closed bool
}

func (s *execIn) Read(p []byte) (n int, err error) {
	for {
		// Return buffered data.
		if len(s.buf) > 0 {
			n = copy(p, s.buf)
			s.buf = s.buf[n:]
			return n, nil
		}
		if s.closed {
			return 0, io.EOF
		}

		// Buffer is empty, wait for new data.
		req, err := s.srv.Recv()
		if err != nil {
			return 0, err
		}

		// Fill the buffer and continue.
		ev := req.GetType().(*protocol.ExecRequest_Stdin).Stdin
		s.buf = ev.GetData()
		if ev.GetClose() {
			s.closed = true
		}
	}
}

// execOut implements io.WriteCloser which writes to the loopback process stdout
// or stderr.
type execOut struct {
	srv  protocol.LoopbackExecService_ExecServer
	ctor func(*protocol.PipeEvent) *protocol.ExecResponse
}

func (s *execOut) Write(p []byte) (n int, err error) {
	if err := s.srv.Send(s.ctor(&protocol.PipeEvent{Data: p})); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *execOut) Close() error {
	return s.srv.Send(s.ctor(&protocol.PipeEvent{Close: true}))
}
