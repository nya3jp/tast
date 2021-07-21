// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"

	"chromiumos/tast/errors"
	"chromiumos/tast/exec"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/shutil"
)

// Cmd represents an external command being prepared or run on a remote host.
//
// This type implements the almost exactly the same interface as Cmd in os/exec.
type Cmd struct {
	// Args holds command line arguments, including the command as Args[0].
	Args []string

	// Dir specifies the working directory of the command.
	// If Dir is the empty string, Run runs the command in the default directory,
	// typically the home directory of the SSH user.
	Dir string

	// Stdin specifies the process's standard input.
	Stdin io.Reader

	// Stdout specifies the process's standard output.
	Stdout io.Writer

	// Stderr specifies the process's standard error.
	Stderr io.Writer

	ssh *Conn

	state                  cmdState
	abort                  chan struct{}  // closed when Abort is called
	log                    bytes.Buffer   // uncaptured stdout/stderr
	stdoutPipe, stderrPipe *io.PipeWriter // set when StdoutPipe/StderrPipe are called
	onceClose              sync.Once      // used to close stdoutPipe/stderrPipe just once
	sess                   *ssh.Session

	// ctx is the context given to Command that specifies the timeout of the external command.
	ctx context.Context
}

// cmdState represents a state of a Cmd. cmdState is used to prevent typical misuse of
// Cmd methods, though it does not catch all concurrent cases.
type cmdState int

const (
	stateNew     cmdState = iota // newly created
	stateStarted                 // after Start is called
	stateClosing                 // after waitAndClose is called
	stateDone                    // after waitAndClose is returned or initialization failed
)

func (s cmdState) String() string {
	switch s {
	case stateNew:
		return "new"
	case stateStarted:
		return "started"
	case stateClosing:
		return "closing"
	case stateDone:
		return "done"
	default:
		return fmt.Sprintf("invalid(%d)", int(s))
	}
}

// RunOption is enum of options which can be passed to Run, Output,
// CombinedOutput and Wait to control precise behavior of them.
type RunOption = exec.RunOption

// DumpLogOnError instructs to dump logs if the executed command fails
// (i.e., exited with non-zero status code).
const DumpLogOnError = exec.DumpLogOnError

func hasOpt(opt RunOption, opts []RunOption) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

var (
	errStdoutSet = errors.New("Stdout was already set")
	errStderrSet = errors.New("Stderr was already set")
	errNotWaited = errors.New("Wait was not yet called")
)

// CommandContext returns the Cmd struct to execute the named program with the given arguments.
//
// It is fine to call this method with nil receiver; subsequent method calls will just fail.
//
// See: https://godoc.org/os/exec#Command
func (s *Conn) CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	return &Cmd{
		Args:  append([]string{name}, args...),
		ssh:   s,
		abort: make(chan struct{}),
		ctx:   ctx,
	}
}

// Run starts the specified command and waits for it to complete.
//
// The command is aborted when ctx's deadline is reached.
//
// See: https://godoc.org/os/exec#Cmd.Run
func (c *Cmd) Run(opts ...RunOption) error {
	if err := c.Start(); err != nil {
		return err
	}

	return c.Wait(opts...)
}

// Output runs the command and returns its standard output.
//
// The command is aborted when ctx's deadline is reached.
//
// See: https://godoc.org/os/exec#Cmd.Output
func (c *Cmd) Output(opts ...RunOption) ([]byte, error) {
	if c.Stdout != nil {
		return nil, errStdoutSet
	}

	var buf bytes.Buffer
	c.Stdout = &buf

	err := c.Run(opts...)
	return buf.Bytes(), err
}

// CombinedOutput runs the command and returns its combined standard output and standard error.
//
// The command is aborted when ctx's deadline is reached.
//
// See: https://godoc.org/os/exec#Cmd.CombinedOutput
func (c *Cmd) CombinedOutput(opts ...RunOption) ([]byte, error) {
	if c.Stdout != nil {
		return nil, errStdoutSet
	}
	if c.Stderr != nil {
		return nil, errStderrSet
	}

	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	err := c.Run(opts...)
	return buf.Bytes(), err
}

// StdinPipe returns a pipe that will be connected to the command's standard input
// when the command starts.
//
// Close the pipe to send EOF to the remote process.
//
// Important difference with os/exec:
//
// The returned pipe is NOT closed automatically. Wait/Run/Output/CombinedOutput
// may block until you close the pipe explicitly.
//
// See: https://godoc.org/os/exec#Cmd.StdinPipe
func (c *Cmd) StdinPipe() (io.WriteCloser, error) {
	if c.state != stateNew {
		return nil, errors.New("stdin must be set up before starting process")
	}
	if c.Stdin != nil {
		return nil, errors.New("stdin is already set")
	}

	r, w := io.Pipe()
	c.Stdin = r
	return w, nil
}

// StdoutPipe returns a pipe that will be connected to the command's standard output
// when the command starts.
//
// The returned pipe is closed automatically when the context deadline is reached,
// Abort is called, or Wait/Run/Output/CombinedOutput sees the command exit.
// Thus it is incorrect to call Wait while reading from the pipe, or to use
// StdoutPipe with Run/Output/CombinedOutput. See the os/exec documentation for
// details.
//
// See: https://godoc.org/os/exec#Cmd.StdoutPipe
func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	if c.state != stateNew {
		return nil, errors.New("stdout must be set up before starting process")
	}
	if c.Stdout != nil {
		return nil, errors.New("stdout is already set")
	}

	r, w := io.Pipe()
	c.Stdout = w
	c.stdoutPipe = w
	return r, nil
}

// StderrPipe returns a pipe that will be connected to the command's standard error
// when the command starts.
//
// The returned pipe is closed automatically when the context deadline is reached,
// Abort is called, or Wait/Run/Output/CombinedOutput sees the command exit.
// Thus it is incorrect to call Wait while reading from the pipe, or to use
// StderrPipe with Run/Output/CombinedOutput. See the os/exec documentation for
// details.
//
// See: https://godoc.org/os/exec#Cmd.StderrPipe
func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	if c.state != stateNew {
		return nil, errors.New("stderr must be set up before starting process")
	}
	if c.Stderr != nil {
		return nil, errors.New("stderr is already set")
	}

	r, w := io.Pipe()
	c.Stderr = w
	c.stderrPipe = w
	return r, nil
}

// Start starts the specified command but does not wait for it to complete.
//
// See: https://godoc.org/os/exec#Cmd.Start
func (c *Cmd) Start() error {
	if c.Stdout == nil {
		c.Stdout = &c.log
	}
	if c.Stderr == nil {
		c.Stderr = &c.log
	}

	if err := c.startSession(c.ctx); err != nil {
		return err
	}

	if err := doAsync(c.ctx, func() error {
		return c.sess.Start(c.buildCmd(c.Dir, c.Args))
	}, func() {
		c.sess.Close()
	}); err != nil {
		c.state = stateDone
		c.closePipes(io.EOF)
		return err
	}
	return nil
}

// Wait waits for the command to exit and waits for any copying to stdin or
// copying from stdout or stderr to complete.
//
// This method can be called only if the command was started by Start. It is an
// error to call this method multiple times, but it will not panic as long as
// it is not called in parallel.
//
// See: https://godoc.org/os/exec#Cmd.Wait
func (c *Cmd) Wait(opts ...RunOption) error {
	if c.state != stateStarted {
		return errors.New("process not active")
	}

	werr := c.waitAndClose(func() error {
		return c.sess.Wait()
	})

	if werr != nil && hasOpt(DumpLogOnError, opts) {
		if err := c.DumpLog(c.ctx); err != nil {
			return fmt.Errorf("BUG: unexpected state %v, want stateDone", c.state)
		}
	}
	return werr
}

// DumpLog logs details of the executed external command, including uncaptured
// output.
//
// This function must be called after Wait.
func (c *Cmd) DumpLog(ctx context.Context) error {
	if c.state != stateDone {
		return errNotWaited
	}
	logging.Info(ctx, "Command: ", shutil.EscapeSlice(c.Args))
	logging.Info(ctx, "Uncaptured output:\n", c.log.String())
	return nil
}

// Abort requests to abort the command execution.
//
// This method does not block, but you still need to call Wait. It is safe to
// call this method while calling Wait/Run/Output/CombinedOutput in another
// goroutine. After calling this method, Wait/Run/Output/CombinedOutput will
// return immediately. This method can be called at most once.
func (c *Cmd) Abort() {
	c.closePipes(errors.New("aborted by client"))
	close(c.abort)
}

// startSession starts a new SSH session and sets c.sess.
func (c *Cmd) startSession(ctx context.Context) error {
	if c.state != stateNew {
		return errors.New("can not start sessions multiple times")
	}
	if c.ssh == nil {
		return errors.New("SSH connection is not available")
	}

	// Set the state early to reject startSession to be called twice.
	c.state = stateStarted

	var sess *ssh.Session

	if err := doAsync(ctx, func() error {
		var err error
		sess, err = c.ssh.cl.NewSession()
		if err != nil {
			return err
		}
		return c.setupSession(sess)
	}, func() {
		if sess != nil {
			sess.Close()
		}
	}); err != nil {
		c.state = stateDone
		c.closePipes(io.EOF)
		return errors.Wrap(err, "failed to create session")
	}

	c.sess = sess
	return nil
}

// setupSession sets up pipes for a new session sess.
func (c *Cmd) setupSession(sess *ssh.Session) error {
	var copiers []func() // functions to run on background goroutines to copy pipe data

	sess.Stdin = c.Stdin

	// When using pipes, make sure to close them to send EOF after copying data.
	// Note that sess.Stdout/Stderr are io.Writer so they're not closed.
	if c.stdoutPipe == nil {
		sess.Stdout = c.Stdout
	} else {
		p, err := sess.StdoutPipe()
		if err != nil {
			return err
		}
		copiers = append(copiers, func() {
			_, err := io.Copy(c.stdoutPipe, p)
			c.stdoutPipe.CloseWithError(err)
		})
	}

	if c.stderrPipe == nil {
		sess.Stderr = c.Stderr
	} else {
		p, err := sess.StderrPipe()
		if err != nil {
			return err
		}
		copiers = append(copiers, func() {
			_, err := io.Copy(c.stderrPipe, p)
			c.stderrPipe.CloseWithError(err)
		})
	}

	// Unlike Cmd in os/exec, x/crypto/ssh isn't concurrent safe if Stdout
	// and Stderr are the same writer.
	if sess.Stdout != nil && interfaceEqual(sess.Stdout, sess.Stderr) {
		w := &safeWriter{w: sess.Stdout}
		sess.Stdout = w
		sess.Stderr = w
	}

	for _, f := range copiers {
		go f()
	}
	return nil
}

// interfaceEqual protects against panics from doing equality tests on
// two interfaces with non-comparable underlying types.
func interfaceEqual(a, b interface{}) bool {
	defer func() {
		recover()
	}()
	return a == b
}

type safeWriter struct {
	w   io.Writer
	mux sync.Mutex
}

func (w *safeWriter) Write(b []byte) (int, error) {
	w.mux.Lock()
	defer w.mux.Unlock()
	return w.w.Write(b)
}

// waitAndClose runs f which waits for the command to finish, and close the
// session.
func (c *Cmd) waitAndClose(f func() error) error {
	if c.state != stateStarted {
		return fmt.Errorf("waitAndClose called in invalid state (%v)", c.state)
	}

	c.state = stateClosing

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	// Cancel the context when Abort is called.
	go func() {
		select {
		case <-c.abort:
			cancel()
		case <-ctx.Done():
		}
	}()

	retErr := doAsync(ctx, f, nil)

	// The remote process exited or timed out. Close pipes before running
	// possibly blocking operations.
	c.closePipes(io.EOF)

	if err := doAsync(ctx, func() error {
		c.sess.Signal(ssh.SIGKILL) // in case the command is still running
		return c.sess.Close()
	}, nil); err != nil && err != io.EOF && retErr == nil { // Close returns io.EOF on success
		retErr = err
	}

	c.state = stateDone
	return retErr
}

// closePipes closes the pipes created by StdoutPipe and StderrPipe.
// It is safe to call this method multiple times concurrently.
func (c *Cmd) closePipes(err error) {
	c.onceClose.Do(func() {
		if c.stdoutPipe != nil {
			c.stdoutPipe.CloseWithError(err)
		}
		if c.stderrPipe != nil {
			c.stderrPipe.CloseWithError(err)
		}
	})
}

// buildCmd builds a shell command in a platform-specific manner.
func (c *Cmd) buildCmd(dir string, args []string) string {
	return c.ssh.platform.BuildShellCommand(dir, args)
}
