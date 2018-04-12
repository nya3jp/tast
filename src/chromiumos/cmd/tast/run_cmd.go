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

	"chromiumos/cmd/tast/run"
	"chromiumos/cmd/tast/timing"

	"github.com/google/subcommands"
)

const (
	baseResultsDir       = "/tmp/tast/results" // base directory under which test results are written
	latestResultsSymlink = "latest"            // symlink in baseResultsDir pointing at latest results

	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information

	localType  = "local"  // -buildtype flag value for local tests
	remoteType = "remote" // -buildtype flag value for remote tests
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	buildType string     // type of tests to build and deploy (either "local" or "remote")
	cfg       run.Config // shared config for running tests
	wrapper   runWrapper // can be set by tests to stub out calls to run package
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
	// TODO(derat): Move this flag (and associated code in runTests) to the run package: https://crbug.com/831849
	f.StringVar(&r.buildType, "buildtype", localType,
		"type of tests to build (\""+localType+"\" or \""+remoteType+"\")")

	td := getTrunkDir()
	r.cfg.SetFlags(f, td)
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
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

	status, results := r.runTests(ctx)
	if len(results) == 0 {
		if status == subcommands.ExitSuccess {
			lg.Logf("No tests matched by pattern(s) %v", r.cfg.Patterns)
			if r.cfg.Build {
				other := localType
				if r.buildType == localType {
					other = remoteType
				}
				lg.Logf("Do you need to pass -buildtype=" + other + "?")
			}
			status = subcommands.ExitFailure
		}
		return status
	}

	if err = r.wrapper.writeResults(ctx, &r.cfg, results); err != nil {
		lg.Log("Failed to write results: ", err)
		if status == subcommands.ExitSuccess {
			status = subcommands.ExitFailure
		}
	}
	return status
}

// runTests executes tests as specified in r.cfg and returns results.
func (r *runCmd) runTests(ctx context.Context) (status subcommands.ExitStatus, results []run.TestResult) {
	if !r.cfg.Build {
		// If we aren't rebuilding a bundle, run both local and remote tests and merge the results.
		if status, results = r.wrapper.local(ctx, &r.cfg); status == subcommands.ExitSuccess {
			var rres []run.TestResult
			status, rres = r.wrapper.remote(ctx, &r.cfg)
			results = append(results, rres...)
		}
		return status, results
	}

	switch r.buildType {
	case localType:
		return r.wrapper.local(ctx, &r.cfg)
	case remoteType:
		return r.wrapper.remote(ctx, &r.cfg)
	default:
		lg.Logf(fmt.Sprintf("Invalid -buildtype %q\n\n%s", r.buildType, r.Usage()))
		return subcommands.ExitUsageError, nil
	}
}
