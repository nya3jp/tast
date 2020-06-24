// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/rpc"
	"chromiumos/tast/ssh"

	"google.golang.org/grpc"
)

// runnerCmd provides common interface to execute a test_runner.
// The APIs this interface provides are based on exec.Cmd.
type runnerCmd interface {
	// SetStdin sets the given stdin as the subprocess's stdin.
	SetStdin(stdin io.Reader)

	// SetStderr sets the given stderr as the subprocess's stderr.
	SetStderr(stderr io.Writer)

	// Output executes the command, and returns its stdout.
	Output() ([]byte, error)
}

type streamableRunnerCmd interface {
	runnerCmd

	Start() error
	Abort()
	Wait(ctx context.Context) error
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
}

type delegateAdaptor struct {
	// r is the reader to get the message from.
	r io.Reader
	// w is the writer to send the message to.
	w io.WriteCloser

	// ech is the channel to send an error on streaming.
	ech     chan error
	waitErr *error

	// conn is the connection to use.
	conn *grpc.ClientConn
}

// newDelegateAdaptor gets conn and its ownership.
// Caller must call close().
func newDelegateAdaptor() *delegateAdaptor {
	return &delegateAdaptor{
		r:   os.Stdin,
		w:   os.Stdout,
		ech: make(chan error, 1),
	}
}

// start invokes a Delegate request, and returns without blocking.
// It invokes a goroutine that redirects the output stream to a.stdout.
// The goroutine sends an error or nil to a.ech when the gRPC call finishes or timeouts, and returns.
// start takes the ownership of conn, and closes it in wait.
func (a *delegateAdaptor) start(ctx context.Context, conn *grpc.ClientConn) error {
	a.conn = conn
	cl := runner.NewTastCoreServiceClient(conn)
	b, err := ioutil.ReadAll(a.r)
	if err != nil {
		return err
	}
	stream, err := cl.Delegate(ctx, &runner.DelegateRequest{Payload: string(b)})
	if err != nil {
		return err
	}
	go func() {
		defer a.w.Close()
		for {
			m, err := stream.Recv()
			if err == io.EOF {
				a.ech <- nil
				break
			}
			if err != nil {
				a.ech <- err
				break
			}
			if _, err := fmt.Fprint(a.w, m.Payload); err != nil {
				a.ech <- err
				break
			}
		}
	}()
	return nil
}

// wait waits for the command to exit and waits for any copying to stdin or
// copying from stdout or stderr to complete.
//
// This method can be called only if the command was started by start. It is an
// error to call this method multiple times, but it will not panic as long as
// it is not called in parallel.
//
// The command is aborted when ctx's deadline is reached. Note that the deadline
// of the context passed to Start also applies.
func (a *delegateAdaptor) wait(ctx context.Context) (retErr error) {
	defer func() {
		if err := a.conn.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if a.waitErr != nil {
		return *a.waitErr
	}
	select {
	case err := <-a.ech:
		a.waitErr = &err
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type localRunnerCmd struct {
	// cmd holds the instance to execute a command on DUT.
	cmd *ssh.Cmd

	// ctx is used to run ssh.Cmd.Start() and ssh.Cmd.Wait().
	ctx context.Context

	// a is the adaptor to send and receive messages.
	// Start() sets a.
	a *delegateAdaptor
}

type localRunner struct {
	conn  *grpc.ClientConn
	close func() error
}

// NewLocalRunner returns gRPC connection to the local runner.
func NewLocalRunner(ctx context.Context, cfg *Config, hst *ssh.Conn) (*localRunner, error) {
	c := localRunnerCommand(ctx, cfg, hst)
	conn, close, err := c.makeClientConn(ctx)
	if err != nil {
		return nil, err
	}
	return &localRunner{
		conn: conn,
		close: func() (retErr error) {
			defer func() {
				if err := close(); err != nil && retErr == nil {
					retErr = err
				}
			}()
			return conn.Close()
		},
	}, nil
}

// Conn returns the grpc client owned by c.
func (c *localRunner) Conn() *grpc.ClientConn {
	return c.conn
}

// Close closes the client.
func (c *localRunner) Close() error {
	return c.close()
}

// makeClientConn creates a client connection from c. close must be called by the caller to stop the gRPC server.
// ctx must be as long as the lifetime of conn.
func (c *localRunnerCmd) makeClientConn(ctx context.Context) (conn *grpc.ClientConn, close func() error, err error) {
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := c.cmd.Start(ctx); err != nil {
		return nil, nil, err
	}
	close = func() error {
		c.cmd.Abort()
		return c.cmd.Wait(ctx)
	}
	conn, err = rpc.NewPipeClientConn(ctx, stdout, stdin)
	if err != nil {
		if err := close(); err != nil {
			logging.ContextLog(ctx, "failed to close: ", err)
		}
		return nil, nil, err
	}
	return conn, close, nil
}

// localRunnerCommand returns a streamableRunnerCmd.
func localRunnerCommand(ctx context.Context, cfg *Config, hst *ssh.Conn) *localRunnerCmd {
	// Set proxy-related environment variables for local_test_runner so it will use them
	// when accessing network.
	execArgs := []string{"env"}
	if cfg.proxy == proxyEnv {
		// Proxy-related variables can be either uppercase or lowercase.
		// See https://golang.org/pkg/net/http/#ProxyFromEnvironment.
		for _, name := range []string{
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
			"http_proxy", "https_proxy", "no_proxy",
		} {
			if val := os.Getenv(name); val != "" {
				execArgs = append(execArgs, fmt.Sprintf("%s=%s", name, val))
			}
		}
	}
	execArgs = append(execArgs, cfg.localRunner, "-grpc")

	return &localRunnerCmd{
		cmd: hst.Command(execArgs[0], execArgs[1:]...),
		ctx: ctx,
		a:   newDelegateAdaptor(),
	}
}

func (c *localRunnerCmd) SetStdin(stdin io.Reader) {
	c.a.r = stdin
}

func (c *localRunnerCmd) SetStderr(stderr io.Writer) {
	c.cmd.Stderr = stderr
}

func (c *localRunnerCmd) Start() error {
	conn, close, err := c.makeClientConn(c.ctx)
	if err != nil {
		return err
	}
	if err = c.a.start(c.ctx, conn); err != nil {
		if err := close(); err != nil {
			logging.ContextLog(c.ctx, "failed to close: ", err)
		}
		return err
	}
	return nil
}

func (c *localRunnerCmd) Abort() {
	c.cmd.Abort()
}

func (c *localRunnerCmd) Wait(ctx context.Context) error {
	if err := c.a.wait(ctx); err != nil {
		return err
	}
	// Ignore context cancelled error, which happens when Abort() has been called.
	c.cmd.Wait(ctx)
	return nil
}

func (c *localRunnerCmd) StdoutPipe() (r io.ReadCloser, _ error) {
	r, c.a.w = io.Pipe()
	return r, nil
}

func (c *localRunnerCmd) StderrPipe() (io.ReadCloser, error) {
	return c.cmd.StderrPipe()
}

func (c *localRunnerCmd) Output() (_ []byte, retErr error) {
	r, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	defer func() {
		c.Abort()
		if err := c.Wait(c.ctx); err != nil && retErr == nil {
			retErr = err
		}
	}()
	return ioutil.ReadAll(r)
}

type remoteRunnerCmd struct {
	*exec.Cmd
}

func remoteRunnerCommand(ctx context.Context, cfg *Config) *remoteRunnerCmd {
	return &remoteRunnerCmd{exec.CommandContext(ctx, cfg.remoteRunner)}
}

func (r *remoteRunnerCmd) SetStdin(stdin io.Reader) {
	r.Stdin = stdin
}

func (r *remoteRunnerCmd) SetStderr(stderr io.Writer) {
	r.Stderr = stderr
}

// runTestRunnerCommand executes the given test_runner r. The test_runner reads
// serialized args from its stdin, then output json serialized value to stdout.
// This function unmarshals the output to out, so the pointer to an appropriate
// struct is expected to be passed via out.
func runTestRunnerCommand(r runnerCmd, args *runner.Args, out interface{}) error {
	args.FillDeprecated()
	stdin, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal args: %v", err)
	}
	r.SetStdin(bytes.NewBuffer(stdin))

	var stderr bytes.Buffer
	r.SetStderr(&stderr)

	b, err := r.Output()
	if err != nil {
		// Append the first line of stderr, which often contains useful info
		// for debugging to users.
		if split := bytes.SplitN(stderr.Bytes(), []byte(","), 2); len(split) > 0 {
			err = fmt.Errorf("%s: %s", err, string(split[0]))
		}
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unmarshal output: %v", err)
	}
	return nil
}
