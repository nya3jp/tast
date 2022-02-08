// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec

import (
	"context"
	"io"
	"os"
	"os/exec"

	"chromiumos/tast/internal/debugger"
)

// ExecCmd represents a local command to execute.
type ExecCmd struct {
	name      string
	baseArgs  []string
	debugPort int
}

var _ Cmd = &ExecCmd{}

// CommandExec constructs a new ExecCmd representing a local command to execute.
func CommandExec(name string, baseArgs ...string) *ExecCmd {
	return &ExecCmd{
		name:     name,
		baseArgs: baseArgs,
	}
}

// DebugCommand returns a version of this command that will run under the debugger.
func (c *ExecCmd) DebugCommand(ctx context.Context, debugPort int) (Cmd, error) {
	if debugPort == 0 {
		return c, nil
	}
	if err := debugger.FindPreemptiveDebuggerErrors(debugPort, false); err != nil {
		return nil, err
	}
	debugEnv := debugger.DlvHostEnv
	if debugger.IsRunningOnDUT() {
		debugEnv = debugger.DlvDUTEnv
	}
	name, baseArgs := debugger.RewriteDebugCommand(debugPort, debugEnv, c.name, c.baseArgs...)
	return &ExecCmd{name: name, baseArgs: baseArgs, debugPort: debugPort}, nil
}

// Run runs a local command synchronously. See Cmd.Run for details.
func (c *ExecCmd) Run(ctx context.Context, extraArgs []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, c.name, append(c.baseArgs, extraArgs...)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Set FD 3 to the real stderr so that the subprocess can write stack
	// traces.
	// TODO(b/189332919): Remove this hack and write stack traces to stderr
	// once we finish migrating to gRPC-based protocol. This hack is needed
	// because JSON-based protocol is designed to write messages to stderr
	// in case of errors and thus Tast CLI consumes stderr.
	cmd.ExtraFiles = []*os.File{os.Stderr}
	cmd.Env = append(os.Environ(), "TAST_B189332919_STACK_TRACE_FD=3")
	debugger.PrintWaitingMessage(ctx, c.debugPort)
	return cmd.Run()
}

// Interact runs a local command asynchronously. See Cmd.Interact for details.
func (c *ExecCmd) Interact(ctx context.Context, extraArgs []string) (p Process, retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()

	cmd := exec.CommandContext(ctx, c.name, append(c.baseArgs, extraArgs...)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	debugger.PrintWaitingMessage(ctx, c.debugPort)

	return &ExecProcess{
		cmd:    cmd,
		cancel: cancel,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// ExecProcess represents a locally running process.
type ExecProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

var _ Process = &ExecProcess{}

// Stdin returns stdin of the process.
func (p *ExecProcess) Stdin() io.WriteCloser { return p.stdin }

// Stdout returns stdout of the process.
func (p *ExecProcess) Stdout() io.ReadCloser { return p.stdout }

// Stderr returns stderr of the process.
func (p *ExecProcess) Stderr() io.ReadCloser { return p.stderr }

// Wait waits for the process to exit. See Process.Wait for details.
func (p *ExecProcess) Wait(ctx context.Context) error {
	exited := make(chan struct{})
	defer close(exited)

	// Cancel the context passed to exec.CommandContext to kill the
	// process.
	go func() {
		select {
		case <-ctx.Done():
		case <-exited:
		}
		p.cancel()
	}()

	return p.cmd.Wait()
}

// ProcessState returns the os.ProcessState object for the process..
func (p *ExecProcess) ProcessState() *os.ProcessState {
	return p.cmd.ProcessState
}
