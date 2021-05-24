// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakesshserver implements a fake SSH server.
package fakesshserver

import (
	"io"
	"net"
	"os/exec"
	"strings"

	"chromiumos/tast/internal/sshtest"
)

// Process implements a simulated process started by a fake SSH server.
type Process func(stdin io.Reader, stdout, stderr io.Writer) int

// Handler receives a command requested by an SSH client and decides whether to
// handle the request.
// If it returns true, a reply is sent to the client indicating that the command
// is accepted, and returned Process is called with stdin/stdout/stderr.
// If it returns false, an unsuccessful reply is sent to the client.
type Handler func(cmd string) (Process, bool)

// ExactMatchHandler constructs a Handler that replies to a command request by
// proc if it exactly matches with cmd.
func ExactMatchHandler(cmd string, proc Process) Handler {
	return func(c string) (Process, bool) {
		if c != cmd {
			return nil, false
		}
		return proc, true
	}
}

// ShellHandler constructs a Handler that replies to a command request by
// running it as is with "sh -c" if its prefix matches with the given prefix.
func ShellHandler(prefix string) Handler {
	return func(c string) (Process, bool) {
		if !strings.HasPrefix(c, prefix) {
			return nil, false
		}
		return func(stdin io.Reader, stdout, stderr io.Writer) int {
			cmd := exec.Command("sh", "-c", c)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			err := cmd.Run()
			if err != nil {
				if xerr, ok := err.(*exec.ExitError); ok {
					return xerr.ExitCode()
				}
				return 255
			}
			return 0
		}, true
	}
}

// Server maintains resources related to a fake SSH server.
type Server struct {
	testData *sshtest.TestData
}

// Start starts a new fake SSH server.
//
// Note: This function does not use the functional option pattern. This package
// is internal to the runtest package so it is easy to rewrite all callers when
// we introduce new configuration values.
func Start(handlers []Handler) *Server {
	server := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		for _, handler := range handlers {
			cmd, ok := handler(req.Cmd)
			if !ok {
				continue
			}
			req.Start(true)
			status := cmd(req, req, req.Stderr())
			req.End(status)
			return
		}
		req.Start(false)
	})
	return &Server{
		testData: server,
	}
}

// Stop stops the fake SSH server.
func (s *Server) Stop() {
	s.testData.Close()
}

// Addr returns the address the server listens to.
func (s *Server) Addr() net.Addr { return s.testData.Srvs[0].Addr() }

// KeyFile returns a path to a file containing the SSH user private key granted
// to access the server.
func (s *Server) KeyFile() string { return s.testData.UserKeyFile }