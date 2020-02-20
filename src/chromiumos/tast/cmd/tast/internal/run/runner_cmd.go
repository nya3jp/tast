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

	"chromiumos/tast/host"
	"chromiumos/tast/internal/runner"
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

type localRunnerCmd struct {
	// cmd holds the instance to execute a command on DUT.
	cmd *host.Cmd

	// ctx is used to run host.Cmd.Start() and host.Cmd.Wait().
	ctx context.Context
}

func localRunnerCommand(ctx context.Context, cfg *Config, hst *host.SSH) *localRunnerCmd {
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

	return &localRunnerCmd{hst.Command(execArgs[0], execArgs[1:]...), ctx}
}

func (r *localRunnerCmd) SetStdin(stdin io.Reader) {
	r.cmd.Stdin = stdin
}

func (r *localRunnerCmd) SetStderr(stderr io.Writer) {
	r.cmd.Stderr = stderr
}

func (r *localRunnerCmd) Output() ([]byte, error) {
	return r.cmd.Output(r.ctx)
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
