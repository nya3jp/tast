// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/cmd/tast/run"
	"chromiumos/cmd/tast/timing"

	"github.com/google/subcommands"
)

const (
	baseResultsDir       = "/tmp/tast/results" // base directory under which test results are written
	latestResultsSymlink = "latest"            // symlink in baseResultsDir pointing at latest results

	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information

	writeResultsTimeout = 15 * time.Second // time reserved for writing results when timeout is set
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	cfg        run.Config // shared config for running tests
	wrapper    runWrapper // can be set by tests to stub out calls to run package
	timeoutSec int        // overall timeout in seconds
}

func newRunCmd() *runCmd {
	return &runCmd{
		cfg:     run.Config{Mode: run.RunTestsMode},
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
	f.IntVar(&r.timeoutSec, "timeout", 0, "run timeout in seconds, or 0 for none")
	r.cfg.SetFlags(f, getTrunkDir())
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	var cancel func()
	if r.timeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(r.timeoutSec)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	defer r.cfg.Close(ctx)

	tl := timing.Log{}
	ctx = timing.NewContext(ctx, &tl)
	st := tl.Start("exec")

	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + r.Usage())
		return subcommands.ExitUsageError
	}

	if r.cfg.ResDir == "" {
		r.cfg.ResDir = filepath.Join(baseResultsDir, time.Now().Format("20060102-150405"))

		link := filepath.Join(baseResultsDir, latestResultsSymlink)
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
	var rctx context.Context
	var rcancel func()
	if dl, ok := ctx.Deadline(); ok && dl.After(time.Now().Add(writeResultsTimeout)) {
		rctx, rcancel = context.WithDeadline(ctx, dl.Add(-writeResultsTimeout))
	} else {
		rctx, rcancel = context.WithCancel(ctx)
	}
	defer rcancel()

	status, results := r.wrapper.run(rctx, &r.cfg)
	complete := status == subcommands.ExitSuccess
	if len(results) == 0 {
		if complete {
			lg.Logf("No tests matched by pattern(s) %v", r.cfg.Patterns)
			lg.Log("Do you need to pass -buildtype=local or -buildtype=remote?")
			status = subcommands.ExitFailure
		}
		return status
	}

	if err = r.wrapper.writeResults(ctx, &r.cfg, results, complete); err != nil {
		lg.Log("Failed to write results: ", err)
		if status == subcommands.ExitSuccess {
			status = subcommands.ExitFailure
		}
	}
	return status
}
