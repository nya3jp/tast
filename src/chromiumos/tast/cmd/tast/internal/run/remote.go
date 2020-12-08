// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/timing"
)

// runRemoteTests runs the remote test runner and reads its output.
func runRemoteTests(ctx context.Context, cfg *Config) ([]*EntityResult, error) {
	cfg.Logger.Status("Running remote tests on target")
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	if err := os.MkdirAll(cfg.remoteOutDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %v", err)
	}
	// At the end of tests remoteOutDir should be empty. Otherwise os.Remove
	// fails and the directory is left for debugging.
	defer os.Remove(cfg.remoteOutDir)

	runTests := func(ctx context.Context, patterns []string) (results []*EntityResult, unstarted []string, err error) {
		return runRemoteTestsOnce(ctx, cfg, patterns)
	}
	beforeRetry := func(ctx context.Context) bool { return true }

	start := time.Now()
	results, err := runTestsWithRetry(ctx, cfg, cfg.testNames, runTests, beforeRetry)
	elapsed := time.Since(start)
	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), elapsed.Round(time.Millisecond))
	return results, err
}

// runRemoteTestsOnce synchronously runs remote_test_runner to run remote tests
// matching the supplied patterns (rather than cfg.Patterns).
//
// Results from started tests and the names of tests that should have been
// started but weren't (in the order in which they should've been run) are
// returned.
func runRemoteTestsOnce(ctx context.Context, cfg *Config, patterns []string) (results []*EntityResult, unstarted []string, err error) {
	ctx, st := timing.Start(ctx, "run_remote_tests_once")
	defer st.End()

	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				FeatureArgs: *featureArgsFromConfig(cfg),
				Patterns:    patterns,
				DataDir:     cfg.remoteDataDir,
				OutDir:      cfg.remoteOutDir,
				Target:      cfg.Target,
				KeyFile:     cfg.KeyFile,
				KeyDir:      cfg.KeyDir,
				TastPath:    exe,
				RunFlags: []string{
					"-build=false",
					"-keyfile=" + cfg.KeyFile,
					"-keydir=" + cfg.KeyDir,
					"-remoterunner=" + cfg.remoteRunner,
					"-remotebundledir=" + cfg.remoteBundleDir,
					"-remotedatadir=" + cfg.remoteDataDir,
					"-localrunner=" + cfg.localRunner,
					"-localbundledir=" + cfg.localBundleDir,
					"-localdatadir=" + cfg.localDataDir,
				},
				LocalBundleDir:    cfg.localBundleDir,
				Devservers:        cfg.devservers,
				TLWServer:         cfg.tlwServerForDUT,
				DUTName:           cfg.Target,
				HeartbeatInterval: heartbeatInterval,
				DownloadMode:      cfg.downloadMode,
			},
			BundleGlob: cfg.remoteBundleGlob(),
		},
	}

	// Backfill deprecated fields in case we're executing an old test runner.
	args.FillDeprecated()

	cmd := remoteRunnerCommand(ctx, cfg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	var stdout, stderr io.Reader
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, nil, err
	}
	if stderr, err = cmd.StderrPipe(); err != nil {
		return nil, nil, err
	}
	stderrReader := newFirstLineReader(stderr)

	cfg.Logger.Logf("Starting %v locally", cmd.Path)
	if err = cmd.Start(); err != nil {
		return nil, nil, err
	}

	if err = json.NewEncoder(stdin).Encode(&args); err != nil {
		return nil, nil, fmt.Errorf("write to stdin: %v", err)
	}
	if err = stdin.Close(); err != nil {
		return nil, nil, fmt.Errorf("close stdin: %v", err)
	}

	results, unstarted, rerr := readTestOutput(ctx, cfg, stdout, os.Rename, nil)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}
