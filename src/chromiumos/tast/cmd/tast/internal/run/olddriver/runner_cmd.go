// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package olddriver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/jsonprotocol"
)

// localTestRunnerEnvVars returns a list of environment variables to be
// passed to the local test runner.
func localTestRunnerEnvVars(cfg *config.Config) []string {
	var execArgs []string
	// The delve debugger attempts to write to a directory not on the stateful partition.
	// This ensures it instead writes to the stateful partition.
	if cfg.DebuggerPort(debugger.LocalTestRunner) > 0 || cfg.DebuggerPort(debugger.LocalBundle) > 0 {
		execArgs = append(execArgs, "XDG_CONFIG_HOME=/mnt/stateful_partition/xdg_config")
	}
	// Set proxy-related environment variables for local_test_runner so it will use them
	// when accessing network.
	if cfg.Proxy() == config.ProxyEnv {
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
	return execArgs
}

// runTestRunnerCommand executes the given test_runner r. The test_runner reads
// serialized args from its stdin, then output json serialized value to stdout.
// This function unmarshals the output to out, so the pointer to an appropriate
// struct is expected to be passed via out.
func runTestRunnerCommand(ctx context.Context, cmd genericexec.Cmd, args *jsonprotocol.RunnerArgs, out interface{}) error {
	args.FillDeprecated()

	stdin, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal runner args: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := cmd.Run(ctx, nil, bytes.NewBuffer(stdin), &stdout, &stderr); err != nil {
		// Append the first line of stderr, which often contains useful info
		// for debugging to users.
		if split := bytes.SplitN(stderr.Bytes(), []byte(","), 2); len(split) > 0 {
			err = fmt.Errorf("%s: %s", err, string(split[0]))
		}
		return err
	}
	if err := json.NewDecoder(&stdout).Decode(out); err != nil {
		return fmt.Errorf("failed to unmarshal runner response: %v", err)
	}
	return nil
}
