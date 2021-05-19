// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// ExecCmd represents a local command to execute.
type ExecCmd struct {
	name     string
	baseArgs []string
}

var _ Cmd = &ExecCmd{}

// CommandExec constructs a new ExecCmd representing a local command to execute.
func CommandExec(name string, baseArgs ...string) *ExecCmd {
	return &ExecCmd{
		name:     name,
		baseArgs: baseArgs,
	}
}

// Run runs a local command synchronously. See Cmd.Run for details.
func (c *ExecCmd) Run(ctx context.Context, extraArgs []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, c.name, append(c.baseArgs, extraArgs...)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
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