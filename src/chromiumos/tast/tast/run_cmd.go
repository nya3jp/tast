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

	"chromiumos/tast/tast/run"
	"chromiumos/tast/tast/timing"

	"github.com/google/subcommands"
)

const (
	baseResultsDir = "/tmp/tast/results"             // base directory under which test results are written
	defaultKeyPath = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout

	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information

	localType  = "local"  // -type flag value for local tests
	remoteType = "remote" // -type flag value for remote tests
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	testType string     // type of tests to run (either "local" or "remote")
	cfg      run.Config // shared config for running tests
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "run tests" }
func (*runCmd) Usage() string {
	return `run <flags> <target> <test1> <test2> ...:
	Runs one or more tests on a remote host.
`
}

func (r *runCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&r.testType, "testtype", "local", "type of tests to run (either \"local\" or \"remote\")")
	r.cfg.SetFlags(f)
	r.cfg.BuildCfg.SetFlags(f, getTrunkDir())
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	tl := timing.Log{}
	ctx = timing.NewContext(ctx, &tl)
	st := tl.Start("exec")

	if len(f.Args()) == 0 {
		lg.Log("Missing target.\n\n" + r.Usage())
		return subcommands.ExitUsageError
	}

	r.cfg.ResDir = filepath.Join(baseResultsDir, time.Now().Format("20060102-150405"))
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

	if r.cfg.KeyFile == "" {
		r.cfg.KeyFile = filepath.Join(getTrunkDir(), defaultKeyPath)
	}
	lg.Debug("Using SSH key ", r.cfg.KeyFile)

	lg.Log("Writing results to ", r.cfg.ResDir)
	switch r.testType {
	case localType:
		return run.Local(ctx, &r.cfg)
	case remoteType:
		return run.Remote(ctx, &r.cfg)
	}
	lg.Logf(fmt.Sprintf("Invalid test type %q\n\n%s", r.testType, r.Usage()))
	return subcommands.ExitUsageError
}
