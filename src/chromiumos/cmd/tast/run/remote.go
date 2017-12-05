// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/timing"

	"github.com/google/subcommands"
)

const (
	remoteTestsPackage = "chromiumos/cmd/remote_tests" // executable package containing remote tests
	remoteTestsFile    = "remote_tests"                // filename for remote test executable
)

// Remote runs remote tests as directed by cfg.
func Remote(ctx context.Context, cfg *Config) subcommands.ExitStatus {
	if cfg.Build && cfg.BuildCfg.Arch == "" {
		var err error
		if cfg.BuildCfg.Arch, err = build.GetLocalArch(); err != nil {
			cfg.Logger.Log("Failed to get local arch: ", err)
			return subcommands.ExitFailure
		}
	}

	bin := cfg.BuildCfg.OutPath(remoteTestsFile)
	if cfg.Build {
		cfg.Logger.Status("Building tests")
		start := time.Now()
		cfg.Logger.Debugf("Building %s from %s to %s", remoteTestsPackage, cfg.BuildCfg.TestWorkspace, bin)
		if out, err := build.BuildTests(ctx, &cfg.BuildCfg, remoteTestsPackage, bin); err != nil {
			cfg.Logger.Logf("Failed building tests: %v\n\n%s", err, out)
			return subcommands.ExitFailure
		}
		cfg.Logger.Logf("Built tests in %0.1f sec", time.Now().Sub(start).Seconds())
	}

	if err := runRemoteTestBinary(ctx, bin, cfg); err != nil {
		cfg.Logger.Log("Failed to run tests: ", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// runRemoteTestBinary runs the binary containing remote tests and reads its output.
func runRemoteTestBinary(ctx context.Context, bin string, cfg *Config) error {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("run_tests")
		defer st.End()
	}

	if cfg.PrintMode != DontPrint {
		args := []string{"-listtests"}
		args = append(args, cfg.Patterns...)
		b, err := exec.Command(bin, args...).Output()
		if err != nil {
			return err
		}
		return printTests(cfg.PrintDest, b, cfg.PrintMode)
	}

	args := []string{"-report", "-target=" + cfg.Target, "-keypath=" + cfg.KeyFile}
	args = append(args, cfg.Patterns...)
	cmd := exec.Command(bin, args...)

	var err error
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return fmt.Errorf("failed to open stdout: %v", err)
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return fmt.Errorf("failed to open stderr: %v", err)
	}
	stderrReader := newFirstLineReader(stderr)

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := readTestOutput(ctx, cfg, stdout, os.Rename); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		ln, _ := stderrReader.getLine(stderrTimeout)
		return fmt.Errorf("%v: %v", err, ln)
	}
	return nil
}
