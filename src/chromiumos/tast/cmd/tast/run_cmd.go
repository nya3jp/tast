// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/internal/xcontext"
)

const (
	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information

	writeResultsTimeout = 15 * time.Second // time reserved for writing results when timeout is set
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	cfg          *config.Config // shared config for running tests
	wrapper      runWrapper     // can be set by tests to stub out calls to run package
	failForTests bool           // exit with 1 if any individual tests fail
	timeout      time.Duration  // overall timeout; 0 if no timeout
}

var _ = subcommands.Command(&runCmd{})

func newRunCmd(trunkDir string) *runCmd {
	return &runCmd{
		cfg:     config.NewConfig(config.RunTestsMode, tastDir, trunkDir),
		wrapper: &realRunWrapper{},
	}
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "run tests" }
func (*runCmd) Usage() string {
	return `Usage: run [flag]... <target> [pattern]...

Run one or more tests on a target device.

The target is an SSH connection spec of the form "[user@]host[:port]".
Patterns are either globs matching test names or a single test attribute
boolean expression in parentheses (e.g. "(informational && !disabled)").

Exits with 0 if all expected tests were executed, even if some of them failed.
Non-zero exit codes indicate high-level issues, e.g. the SSH connection to the
target was lost. Callers should examine results.json or streamed_results.jsonl
for failing tests. -failfortests can be supplied to override this behavior.

`
}

func (r *runCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&r.failForTests, "failfortests", false, "exit with 1 if any tests fail")
	f.Var(command.NewDurationFlag(time.Second, &r.timeout, ctxutil.MaxTimeout), "timeout", "run timeout in seconds")
	r.cfg.SetFlags(f)
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	ctx, cancel := xcontext.WithTimeout(ctx, r.timeout, errors.Errorf("%v: global timeout reached (%v)", context.DeadlineExceeded, r.timeout))
	defer cancel(context.Canceled)

	var state config.State
	defer state.Close(ctx)

	tl := timing.NewLog()
	ctx = timing.NewContext(ctx, tl)
	ctx, st := timing.Start(ctx, "exec")

	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + r.Usage())
		return subcommands.ExitUsageError
	}

	updateLatest := r.cfg.ResDir == ""

	if err := r.cfg.DeriveDefaults(); err != nil {
		lg.Log("Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}

	if err := os.MkdirAll(r.cfg.ResDir, 0755); err != nil {
		lg.Log(err)
		return subcommands.ExitFailure
	}

	// Update the "latest" symlink if the default result directory is used.
	if updateLatest {
		link := filepath.Join(filepath.Dir(r.cfg.ResDir), "latest")
		os.Remove(link)
		if err := os.Symlink(filepath.Base(r.cfg.ResDir), link); err != nil {
			lg.Log("Failed to create results symlink: ", err)
		}
	}

	// Write the timing log after the command finishes.
	defer func() {
		st.End()
		f, err := os.Create(filepath.Join(r.cfg.ResDir, timingLogName))
		if err != nil {
			lg.Log(err)
			return
		}
		defer f.Close()
		if err := tl.WritePretty(f); err != nil {
			lg.Log(err)
		}
	}()

	// Log the full output of the command to disk.
	fullLog, err := os.Create(filepath.Join(r.cfg.ResDir, fullLogName))
	if err != nil {
		lg.Log(err)
		return subcommands.ExitFailure
	}
	if err = lg.AddWriter(fullLog, true); err != nil {
		lg.Log(err)
		return subcommands.ExitFailure
	}
	defer func() {
		if err := lg.RemoveWriter(fullLog); err != nil {
			lg.Log(err)
		}
		fullLog.Close()
	}()

	lg.Log("Command line: ", strings.Join(os.Args, " "))
	r.cfg.Target = f.Args()[0]
	r.cfg.Patterns = f.Args()[1:]
	r.cfg.Logger = lg

	if r.cfg.KeyFile != "" {
		lg.Debug("Using SSH key ", r.cfg.KeyFile)
	}
	if r.cfg.KeyDir != "" {
		lg.Debug("Using SSH dir ", r.cfg.KeyDir)
	}
	lg.Log("Writing results to ", r.cfg.ResDir)

	// If a deadline is set, reserve a bit of time to write results and collect system info.
	// Skip doing this if a very-short timeout was set, since it's confusing to get an immediate timeout in that case.
	var wrt time.Duration
	if r.timeout > 2*writeResultsTimeout {
		wrt = writeResultsTimeout
	}
	dl, _ := ctx.Deadline() // deadline is always set above
	rctx, rcancel := xcontext.WithDeadline(ctx, dl.Add(-wrt), errors.Errorf("%v: global timeout reached (%v)", context.DeadlineExceeded, r.timeout-wrt))
	defer rcancel(context.Canceled)

	if err := run.SetupGrpcServices(rctx, r.cfg, &state); err != nil {
		lg.Logf("Failed to set up GRPC servers: %v", err)
		return subcommands.ExitFailure
	}
	if err := run.ResolveHosts(rctx, r.cfg, &state); err != nil {
		lg.Logf("Failed to resolve hosts: %v", err)
		return subcommands.ExitFailure
	}

	status, results := r.wrapper.run(rctx, r.cfg, &state)
	allTestsRun := status.ExitCode == subcommands.ExitSuccess
	if len(results) == 0 && len(r.cfg.TestNamesToSkip) == 0 && allTestsRun {
		lg.Logf("No tests matched by pattern(s) %v", r.cfg.Patterns)
		return subcommands.ExitFailure
	}

	// Write results as long as we at least tried to start executing tests.
	// This step collects system information (e.g. logs and crashes), so we still want it to run
	// if testing was interrupted, or even if no tests started due to the DUT not becoming ready:
	// https://crbug.com/928445
	if !status.FailedBeforeRun {
		if err = r.wrapper.writeResults(ctx, r.cfg, &state, results, allTestsRun); err != nil {
			msg := fmt.Sprintf("Failed to write results: %v", err)
			if status.ExitCode == subcommands.ExitSuccess {
				status = run.Status{ExitCode: subcommands.ExitFailure, ErrorMsg: msg}
			} else {
				// Treat the earlier error as the "main" one, but also log the new one.
				lg.Log(msg)
			}
		}

		// Log the first line of the error as the last line of output to make it easy to see.
		if status.ExitCode != subcommands.ExitSuccess {
			if lines := strings.SplitN(status.ErrorMsg, "\n", 2); len(lines) >= 1 {
				lg.Log(lines[0])
			}
		}
	}

	// If we would otherwise report success (indicating that we executed all tests) but
	// -failfortests was passed (indicating that 1 should be returned for individual test failures),
	// then we need to examine test results.
	if status.ExitCode == subcommands.ExitSuccess && r.failForTests {
		for _, res := range results {
			if len(res.Errors) > 0 {
				status.ExitCode = subcommands.ExitFailure
				break
			}
		}
	}

	return status.ExitCode
}
