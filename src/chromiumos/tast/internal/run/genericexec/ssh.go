// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package genericexec

import (
	"bufio"
	"bytes"
	"context"
	"io"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/ssh"
)

// SSHCmd represents a remote command to execute via SSH.
type SSHCmd struct {
	conn      *ssh.Conn
	name      string
	baseArgs  []string
	debugPort int
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

// Keep attempting to kill the debugger. It's possible that kill will fail
// with "no such process", so just do an || true to ensure it doesn't fail.
const killDebuggerCommand = `while pid=$(pgrep ^dlv$); do kill "${pid}" || true; done`

// DebugCommand returns a version of this command that will run under the debugger.
func (c *SSHCmd) DebugCommand(ctx context.Context, debugPort int) (Cmd, error) {
	if debugPort == 0 {
		return c, nil
	}
	if err := debugger.FindPreemptiveDebuggerErrors(debugPort, true); err != nil {
		return nil, err
	}
	killCurrentDebuggerCmd := SSHCmd{conn: c.conn, name: "sh", baseArgs: []string{"-c", killDebuggerCommand}}
	var stderr bytes.Buffer
	if err := killCurrentDebuggerCmd.Run(ctx, nil, nil, nil, bufio.NewWriter(&stderr)); err != nil {
		return nil, errors.Errorf("Failed to kill the current debugger. stderr: %s. Error: %+v", stderr.String(), err)
	}
	name, baseArgs := debugger.RewriteDebugCommand(debugPort, debugger.DlvDUTEnv, c.name, c.baseArgs...)
	return &SSHCmd{conn: c.conn, name: name, baseArgs: baseArgs, debugPort: debugPort}, nil
}

// Run runs a remote command synchronously. See Cmd.Run for details.
func (c *SSHCmd) Run(ctx context.Context, extraArgs []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := c.conn.CommandContext(ctx, c.name, append(c.baseArgs, extraArgs...)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	debugger.PrintWaitingMessage(ctx, c.debugPort)
	return cmd.Run()
}

// Interact runs a remote command asynchronously. See Cmd.Interact for details.
func (c *SSHCmd) Interact(ctx context.Context, extraArgs []string) (p Process, retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()
	cmd := c.conn.CommandContext(ctx, c.name, append(c.baseArgs, extraArgs...)...)
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

	// Start a gorountine to close stdin when ctx is canceled.
	go func() {
		<-ctx.Done()
		stdin.Close()
	}()

	return &SSHProcess{
		cmd:    cmd,
		cancel: cancel,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// SSHProcess represents a remotely running process over SSH.
type SSHProcess struct {
	cmd    *ssh.Cmd
	cancel context.CancelFunc
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
