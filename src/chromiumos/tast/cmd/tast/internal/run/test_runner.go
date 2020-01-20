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
	"os"
	"os/exec"
	"time"

	"chromiumos/tast/host"
	"chromiumos/tast/internal/runner"
)

// cmd provides common interface to execute a command.
type cmd interface {
	// SetStdin sets the given stdin as the subprocess's stdin.
	SetStdin(stdin io.Reader)

	// StdoutPipe returns a Reader to read the data from subprocess's stdout.
	StdoutPipe() (io.ReadCloser, error)

	// StderrPipe returns a Reader to read the data from subprocess's stderr.
	StderrPipe() (io.ReadCloser, error)

	// Start begins the subprocess.
	Start() error

	// Wait waits for the termination of the subprocess.
	Wait() error

	// Output executes the command, and returns its stdout.
	Output() ([]byte, error)
}

type localRunner struct {
	*host.Cmd

	// ctx is used to run host.Cmd.Start() and host.Cmd.Wait().
	ctx context.Context

	// waitTimeout is the duriation to wait for the subprocess.
	waitTimeout time.Duration
}

func newLocalRunner(ctx context.Context, cfg *Config, hst *host.SSH) *localRunner {
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
	execArgs = append(execArgs, cfg.localRunner)

	// Calculate the timeout duration for Wait().
	const defaultWaitTimeout = 10 * time.Second
	timeout := defaultWaitTimeout
	if cfg.localRunnerWaitTimeout > 0 {
		timeout = cfg.localRunnerWaitTimeout
	}

	return &localRunner{hst.Command(execArgs[0], execArgs[1:]...), ctx, timeout}
}

func (r *localRunner) SetStdin(stdin io.Reader) {
	r.Stdin = stdin
}

func (r *localRunner) Start() error {
	return r.Cmd.Start(r.ctx)
}

func (r *localRunner) Wait() error {
	ctx, cancel := context.WithTimeout(r.ctx, r.waitTimeout)
	defer cancel()
	if err := r.Cmd.Wait(ctx); err != nil {
		r.Cmd.Abort()
		r.Cmd.Wait(r.ctx)
		return err
	}
	return nil
}

func (r *localRunner) Output() ([]byte, error) {
	return r.Cmd.Output(r.ctx)
}

type remoteRunner struct {
	*exec.Cmd
}

func newRemoteRunner(ctx context.Context, cfg *Config) *remoteRunner {
	return &remoteRunner{exec.CommandContext(ctx, cfg.remoteRunner)}
}

func (r *remoteRunner) SetStdin(stdin io.Reader) {
	r.Stdin = stdin
}

// runTestRunnerCommand executes the given test_runner r. The test_runner reads
// serialized args from its stdin, then output json serialized value to stdout.
// This function unmarshals the output to out, so the pointer to an appropriate
// struct is expected to be passed via out.
func runTestRunnerCommand(r cmd, args *runner.Args, out interface{}) error {
	args.FillDeprecated()
	stdin, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal args: %v", err)
	}
	r.SetStdin(bytes.NewBuffer(stdin))

	b, err := r.Output()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unmarshal output: %v", err)
	}
	return nil
}
