// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/bundle"
	"chromiumos/tast/runner"
	"chromiumos/tast/timing"
)

const (
	remoteBundlePkgPathPrefix = "chromiumos/tast/remote/bundles" // Go package path prefix for test bundles

	// remoteBundleBuildSubdir is a subdirectory used for compiled remote test bundles.
	// Bundles are placed here rather than in the top-level build artifacts dir so that
	// local and remote bundles with the same name won't overwrite each other.
	remoteBundleBuildSubdir = "remote_bundles"
)

// remote runs remote tests as directed by cfg and returns the command's exit status.
// If non-nil, the returned results may be passed to WriteResults.
func remote(ctx context.Context, cfg *Config) (Status, []TestResult) {
	start := time.Now()

	// Skip remote tests if -build=true and there is no corresponding remote bundle.
	if cfg.build {
		if _, err := os.Stat(filepath.Join(cfg.remoteBundleDir, cfg.buildBundle)); os.IsNotExist(err) {
			return successStatus, nil
		}
	}

	if err := getSoftwareFeatures(ctx, cfg); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get DUT software features: %v", err), nil
	}
	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to get initial sysinfo: %v", err), nil
	}

	cfg.startedRun = true
	results, err := runRemoteRunner(ctx, cfg)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to run tests: %v", err), results
	}
	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))
	return successStatus, results
}

// runRemoteRunner runs the remote test runner and reads its output.
func runRemoteRunner(ctx context.Context, cfg *Config) ([]TestResult, error) {
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	var bundleGlob string
	if cfg.build {
		bundleGlob = filepath.Join(cfg.remoteBundleDir, cfg.buildBundle)
	} else {
		bundleGlob = filepath.Join(cfg.remoteBundleDir, "*")
	}

	var args runner.Args
	switch cfg.mode {
	case RunTestsMode:
		args = runner.Args{
			Mode: runner.RunTestsMode,
			RunTests: &runner.RunTestsArgs{
				BundleArgs: bundle.RunTestsArgs{
					Patterns: cfg.Patterns,
					DataDir:  cfg.remoteDataDir,
					TestVars: cfg.testVars,
					Target:   cfg.Target,
					KeyFile:  cfg.KeyFile,
					KeyDir:   cfg.KeyDir,
					TastPath: exe,
					RunFlags: []string{
						"-keyfile=" + cfg.KeyFile,
						"-keydir=" + cfg.KeyDir,
						"-remoterunner=" + cfg.remoteRunner,
						"-remotebundledir=" + cfg.remoteBundleDir,
						"-remotedatadir=" + cfg.remoteDataDir,
					},
				},
				BundleGlob: bundleGlob,
			},
		}
		setRunnerTestDepsArgs(cfg, &args)

		// Create an output directory within the results dir so we can just move
		// it to its final destination later.
		outDir, err := ioutil.TempDir(cfg.ResDir, "out.")
		if err != nil {
			return nil, fmt.Errorf("failed to create output dir: %v", err)
		}
		args.RunTests.BundleArgs.OutDir = outDir
	case ListTestsMode:
		args = runner.Args{
			Mode: runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{
				BundleArgs: bundle.ListTestsArgs{Patterns: cfg.Patterns},
				BundleGlob: bundleGlob,
			},
		}
	}

	// Backfill deprecated fields in case we're executing an old test runner.
	args.FillDeprecated()

	cmd := exec.Command(cfg.remoteRunner)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return nil, err
	}
	stderrReader := newFirstLineReader(stderr)

	cfg.Logger.Logf("Starting %v locally", cmd.Path)
	if err = cmd.Start(); err != nil {
		return nil, err
	}

	if err = json.NewEncoder(stdin).Encode(&args); err != nil {
		return nil, fmt.Errorf("write to stdin: %v", err)
	}
	if err = stdin.Close(); err != nil {
		return nil, fmt.Errorf("close stdin: %v", err)
	}

	var results []TestResult
	var rerr error
	switch cfg.mode {
	case ListTestsMode:
		results, rerr = readTestList(stdout)
	case RunTestsMode:
		results, _, rerr = readTestOutput(ctx, cfg, stdout, os.Rename, nil)
	}

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil {
		return results, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, rerr
}
