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

	localType  = "local"  // -type flag value for local tests
	remoteType = "remote" // -type flag value for remote tests
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	testType  string     // type of tests to run (either "local" or "remote")
	checkDeps bool       // true if test package's dependencies should be checked before building
	cfg       run.Config // shared config for running tests
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "run tests" }
func (*runCmd) Usage() string {
	return `run <flags> <target> <pattern> <pattern> ...:
	Runs one or more tests on a remote host.
`
}

func (r *runCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&r.testType, "type", "local", "type of tests to run (either \"local\" or \"remote\")")
	f.BoolVar(&r.checkDeps, "checkdeps", true, "checks test package's dependencies before building")

	td := getTrunkDir()
	r.cfg.SetFlags(f, td)
	r.cfg.BuildCfg.SetFlags(f, td)
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
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

	lg.Debug("Using SSH key ", r.cfg.KeyFile)
	lg.Log("Writing results to ", r.cfg.ResDir)

	var status subcommands.ExitStatus
	var results []run.TestResult

	switch r.testType {
	case localType:
		if r.cfg.Build && r.checkDeps {
			r.cfg.BuildCfg.PortagePkg = fmt.Sprintf("chromeos-base/tast-local-tests-%s-9999", r.cfg.BuildBundle)
		}
		status, results = run.Local(ctx, &r.cfg)
	case remoteType:
		if r.cfg.Build && r.checkDeps {
			r.cfg.BuildCfg.PortagePkg = fmt.Sprintf("chromeos-base/tast-remote-tests-%s-9999", r.cfg.BuildBundle)
		}
		status, results = run.Remote(ctx, &r.cfg)
	default:
		lg.Logf(fmt.Sprintf("Invalid test type %q\n\n%s", r.testType, r.Usage()))
		return subcommands.ExitUsageError
	}

	if len(results) > 0 {
		if err = run.WriteResults(&r.cfg, results); err != nil {
			lg.Log("Failed to write results: ", err)
			if status == subcommands.ExitSuccess {
				status = subcommands.ExitFailure
			}
		}
	}
	return status
}
