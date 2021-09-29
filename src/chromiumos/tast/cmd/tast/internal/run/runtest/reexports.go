// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"chromiumos/tast/cmd/tast/internal/run/runtest/internal/fakerunner"
	"chromiumos/tast/internal/fakesshserver"
)

// Runner represents a fake test runner.
type Runner = fakerunner.Runner

// SSHServer maintains resources related to a fake SSH server.
type SSHServer = fakesshserver.Server

// SSHHandler receives a command requested by an SSH client and decides whether
// to handle the request.
// If it returns true, a reply is sent to the client indicating that the command
// is accepted, and returned Process is called with stdin/stdout/stderr.
// If it returns false, an unsuccessful reply is sent to the client.
type SSHHandler = fakesshserver.Handler

// SSHProcess implements a simulated process started by a fake SSH server.
type SSHProcess = fakesshserver.Process

// ExactMatchHandler constructs an SSHHandler that replies to a command request
// by proc if it exactly matches with cmd.
func ExactMatchHandler(cmd string, proc SSHProcess) SSHHandler {
	return fakesshserver.ExactMatchHandler(cmd, proc)
}

// ShellHandler constructs an SSHHandler that replies to a command request by
// running it as is with "sh -c" if its prefix matches with the given prefix.
func ShellHandler(prefix string) SSHHandler {
	return fakesshserver.ShellHandler(prefix)
}
