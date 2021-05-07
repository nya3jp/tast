// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec

import (
	"context"
	"io"

	"chromiumos/tast/ssh"
)

// SSHCmd represents a remote command to execute via SSH.
type SSHCmd struct {
	conn     *ssh.Conn
	name     string
	baseArgs []string
}

var _ Cmd = &SSHCmd{}

// CommandSSH constructs a new SSHCmd representing a remote command to execute
// via SSH.
func CommandSSH(conn *ssh.Conn, name string, baseArgs ...string) *SSHCmd {
	return &SSHCmd{
		conn:     conn,
		name:     name,
		baseArgs: baseArgs,
	}
}

// Run runs a remote command synchronously. See Cmd.Run for details.
func (c *SSHCmd) Run(ctx context.Context, extraArgs []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := c.conn.Command(c.name, append(c.baseArgs, extraArgs...)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run(ctx)
}

// Interact runs a remote command asynchronously. See Cmd.Interact for details.
func (c *SSHCmd) Interact(ctx context.Context, extraArgs []string) (Process, error) {
	cmd := c.conn.Command(c.name, append(c.baseArgs, extraArgs...)...)
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
	if err := cmd.Start(ctx); err != nil {
		return nil, err
	}

	// Start a gorountine to close stdin when ctx is canceled.
	go func() {
		<-ctx.Done()
		stdin.Close()
	}()

	return &SSHProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// SSHProcess represents a remotely running process over SSH.
type SSHProcess struct {
	cmd    *ssh.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

var _ Process = &SSHProcess{}

// Stdin returns stdin of the process.
func (p *SSHProcess) Stdin() io.WriteCloser { return p.stdin }

// Stdout returns stdout of the process.
func (p *SSHProcess) Stdout() io.ReadCloser { return p.stdout }

// Stderr returns stderr of the process.
func (p *SSHProcess) Stderr() io.ReadCloser { return p.stderr }

// Wait waits for the process to exit. See Process.Wait for details.
func (p *SSHProcess) Wait(ctx context.Context) error {
	return p.cmd.Wait(ctx)
}
