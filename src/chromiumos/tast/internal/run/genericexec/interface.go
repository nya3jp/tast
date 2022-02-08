// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec

import (
	"context"
	"io"
)

// Cmd is a common interface abstracting an external command to execute.
type Cmd interface {
	// Run runs an external command synchronously.
	//
	// extraArgs is appended to the base arguments passed to the constructor
	// of Cmd. stdin specifies the data sent to the standard input of the
	// process. The standard output/error of the process are written to
	// stdout/stderr.
	Run(ctx context.Context, extraArgs []string, stdin io.Reader, stdout, stderr io.Writer) error

	// Interact starts an external command asynchronously.
	//
	// extraArgs is appended to the base arguments passed to the constructor
	// of Cmd. Returned Process can be used to interact with the new
	// subprocess.
	//
	// When ctx is canceled, the subprocess is killed by a signal, or
	// stdin of the subprocess is closed, depending on implementation.
	// Therefore Interact is safe to be used only with external commands
	// that exit on closing stdin.
	Interact(ctx context.Context, extraArgs []string) (Process, error)

	// DebugCommand returns a new command that runs the existing command under a
	// debugger waiting on port debugPort, if debugPort is non-zero.
	// It will also ensure that the command is runnable, such as by killing
	// the old debugger.
	DebugCommand(ctx context.Context, debugPort int) (Cmd, error)
}

// Process is a common interface abstracting a running external process.
type Process interface {
	// Stdin returns stdin of the process.
	Stdin() io.WriteCloser

	// Stdout returns stdout of the process.
	Stdout() io.ReadCloser

	// Stderr returns stderr of the process.
	Stderr() io.ReadCloser

	// Wait waits for the process to exit.
	//
	// Wait also releases resources associated to the process, so it must
	// be always called when you are done with it.
	//
	// Upon Wait finishes, io.ReadCloser returned by Stdout and Stderr
	// might be closed. This means that it is wrong to call Wait before
	// finishing to read necessary data from stdout/stderr.
	//
	// When ctx is canceled, the subprocess is killed by a signal, or
	// stdin of the subprocess is closed, depending on implementation.
	Wait(ctx context.Context) error
}
