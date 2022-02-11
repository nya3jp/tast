// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/internal/xcontext"
)

const (
	fullLogName   = "full.txt"    // file in runConfig.resDir containing full output
	timingLogName = "timing.json" // file in runConfig.resDir containing timing information
)

// runCmd implements subcommands.Command to support running tests.
type runCmd struct {
	cfg          *config.MutableConfig // shared config for running tests
	wrapper      runWrapper            // can be set by tests to stub out calls to run package
	failForTests bool                  // exit with 1 if any individual tests fail
	timeout      time.Duration         // overall timeout; 0 if no timeout
}

var _ = subcommands.Command(&runCmd{})

func newRunCmd(trunkDir string) *runCmd {
	return &runCmd{
		cfg:     config.NewMutableConfig(config.RunTestsMode, tastDir, trunkDir),
		wrapper: &realRunWrapper{},
	}
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "run tests" }
func (*runCmd) Usage() string {
	return `Usage: run [flag]... <target> [pattern]...

Description:
    Runs the tast tests on the target based on the pattern provided.
    Exits with 0 if all expected tests were executed, even if some of them failed.
    Non-zero exit codes indicate high-level issues, e.g. the SSH connection to the
    target was lost. Callers should examine results.json or streamed_results.jsonl
    for failing tests. -failfortests can be supplied to override this behavior.

Target:
    The target is an SSH connection spec of the form "[user@]host[:port]".

Pattern:
    Patterns are either globs matching test names or a single test attribute
    boolean expression in parentheses.          
  
    To run tests based attributes pattern, mention single argument surrounded by parentheses. Example:

        $ tast run <target> '(("dep:chrome" || "dep:android") && !informational)'
    
    To run tests based on test name wildcard pattern, use *. Example:

        $ tast run <target> 'ui*' 'wilco*'

    To run specific tests mention them separated by space. Example:

        $ tast run <target>  example.ServoEcho ui.ZoomConfCUJ.basic_large

Flag:
`
}

func (r *runCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&r.failForTests, "failfortests", false, "exit with 1 if any tests fail")
	f.Var(command.NewDurationFlag(time.Second, &r.timeout, ctxutil.MaxTimeout), "timeout", "run timeout in seconds")
	r.cfg.SetFlags(f)
}

func (r *runCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	ctx, cancel := xcontext.WithTimeout(ctx, r.timeout, errors.Errorf("%v: global timeout reached (%v)", context.DeadlineExceeded, r.timeout))
	defer cancel(context.Canceled)

	var state config.DeprecatedState

	tl := timing.NewLog()
	ctx = timing.NewContext(ctx, tl)
	ctx, st := timing.Start(ctx, "exec")

	if len(f.Args()) == 0 {
		logging.Info(ctx, "Missing target.\n\n"+r.Usage())
		return subcommands.ExitUsageError
	}

	updateLatest := r.cfg.ResDir == ""

	if err := r.cfg.DeriveDefaults(); err != nil {
		logging.Info(ctx, "Failed to derive defaults: ", err)
		return subcommands.ExitUsageError
	}

	if err := os.MkdirAll(r.cfg.ResDir, 0755); err != nil {
		logging.Info(ctx, err)
		return subcommands.ExitFailure
	}

	// Update the "latest" symlink if the default result directory is used.
	if updateLatest {
		link := filepath.Join(filepath.Dir(r.cfg.ResDir), "latest")
		os.Remove(link)
		if err := os.Symlink(filepath.Base(r.cfg.ResDir), link); err != nil {
			logging.Info(ctx, "Failed to create results symlink: ", err)
		}
	}

	// Write the timing log after the command finishes.
	defer func() {
		st.End()
		f, err := os.Create(filepath.Join(r.cfg.ResDir, timingLogName))
		if err != nil {
			logging.Info(ctx, err)
			return
		}
		defer f.Close()
		if err := tl.WritePretty(f); err != nil {
			logging.Info(ctx, err)
		}
	}()

	// Log the full output of the command to disk.
	fullLog, err := os.Create(filepath.Join(r.cfg.ResDir, fullLogName))
	if err != nil {
		logging.Info(ctx, err)
		return subcommands.ExitFailure
	}
	defer fullLog.Close()

	logger := logging.NewSinkLogger(logging.LevelDebug, true, logging.NewWriterSink(fullLog))
	ctx = logging.AttachLogger(ctx, logger)

	logging.Info(ctx, "Command line: ", strings.Join(os.Args, " "))
	r.cfg.Target = f.Args()[0]
	r.cfg.Patterns = f.Args()[1:]

	if r.cfg.KeyFile != "" {
		logging.Debug(ctx, "Using SSH key ", r.cfg.KeyFile)
	}
	if r.cfg.KeyDir != "" {
		logging.Debug(ctx, "Using SSH dir ", r.cfg.KeyDir)
	}
	logging.Info(ctx, "Writing results to ", r.cfg.ResDir)

	results, runErr := r.wrapper.run(ctx, r.cfg.Freeze(), &state)

	if runErr == nil && len(results) == 0 && len(state.TestNamesToSkip) == 0 {
		runErr = errors.Errorf("no tests matched by pattern(s) %v", r.cfg.Patterns)
	}

	if runErr != nil {
		logging.Infof(ctx, "Failed to run tests: %v", runErr)
		return subcommands.ExitFailure
	}

	// If we would otherwise report success (indicating that we executed all tests) but
	// -failfortests was passed (indicating that 1 should be returned for individual test failures),
	// then we need to examine test results.
	if r.failForTests {
		for _, res := range results {
			if len(res.Errors) > 0 {
				return subcommands.ExitFailure
			}
		}
	}

	return subcommands.ExitSuccess
}
