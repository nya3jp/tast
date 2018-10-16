// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/subcommands"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/run"
	"chromiumos/cmd/tast/timing"
	"chromiumos/tast/command"
	"chromiumos/tast/ctxutil"
)

const (
	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information

	writeResultsTimeout = 15 * time.Second // time reserved for writing results when timeout is set
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	cfg     *run.Config   // shared config for running tests
	wrapper runWrapper    // can be set by tests to stub out calls to run package
	timeout time.Duration // overall timeout; 0 if no timeout
}

func newRunCmd() *runCmd {
	return &runCmd{
		cfg:     run.NewConfig(run.RunTestsMode, tastDir, trunkDir()),
		wrapper: &realRunWrapper{},
	}
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "run tests" }
func (*runCmd) Usage() string {
	return `run <flags> <target> <pattern> <pattern> ...:
	Runs one or more tests on a remote host.
`
}

func (r *runCmd) SetFlags(f *flag.FlagSet) {
	f.Var(command.NewDurationFlag(time.Second, &r.timeout, 0), "timeout", "run timeout in seconds, or 0 for none")
	r.cfg.SetFlags(f)
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	lg, ok := logging.FromContext(ctx)
	if !ok {
		panic("logger not attached to context")
	}

	ctx, cancel := ctxutil.OptionalTimeout(ctx, r.timeout) // zero or negative ignored
	defer cancel()

	defer r.cfg.Close(ctx)

	tl := timing.Log{}
	ctx = timing.NewContext(ctx, &tl)
	st := tl.Start("exec")

	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + r.Usage())
		return subcommands.ExitUsageError
	}

	if err := r.cfg.DeriveDefaults(); err != nil {
		lg.Log("Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}

	if r.cfg.ResDir == "" {
		baseResultsDir := filepath.Join(tastDir, "results")
		r.cfg.ResDir = filepath.Join(baseResultsDir, time.Now().Format("20060102-150405"))

		link := filepath.Join(baseResultsDir, "latest")
		os.Remove(link)
		if err := os.Symlink(filepath.Base(r.cfg.ResDir), link); err != nil {
			lg.Log("Failed to create results symlink: ", err)
		}
	}
	if err := os.MkdirAll(r.cfg.ResDir, 0755); err != nil {
		lg.Log(err)
		return subcommands.ExitFailure
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
		if err := tl.Write(f); err != nil {
			lg.Log(err)
		}
	}()

	// Log the full output of the command to disk.
	fullLog, err := os.Create(filepath.Join(r.cfg.ResDir, fullLogName))
	if err != nil {
		lg.Log(err)
		return subcommands.ExitFailure
	}
	if err = lg.AddWriter(fullLog, log.LstdFlags); err != nil {
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
	rctx, rcancel := ctxutil.Shorten(ctx, wrt)
	defer rcancel()

	status, results := r.wrapper.run(rctx, r.cfg)
	allTestsRun := status.ExitCode == subcommands.ExitSuccess
	if len(results) == 0 {
		if allTestsRun {
			lg.Logf("No tests matched by pattern(s) %v", r.cfg.Patterns)
			lg.Log("Do you need to pass -buildtype=local or -buildtype=remote?")
			status.ExitCode = subcommands.ExitFailure
		}
		return status.ExitCode
	}

	if err = r.wrapper.writeResults(ctx, r.cfg, results, allTestsRun); err != nil {
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

	return status.ExitCode
}
