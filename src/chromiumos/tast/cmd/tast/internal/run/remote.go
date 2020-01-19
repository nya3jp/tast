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
	"path/filepath"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/timing"
)

// runRemoteTests runs the remote test runner and reads its output.
func runRemoteTests(ctx context.Context, cfg *Config) ([]TestResult, error) {
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	start := time.Now()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				Patterns: cfg.Patterns,
				TestVars: cfg.testVars,
				DataDir:  cfg.remoteDataDir,
				OutDir:   cfg.remoteOutDir,
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
				LocalBundleDir:    cfg.localBundleDir,
				Devservers:        cfg.devservers,
				HeartbeatInterval: heartbeatInterval,
			},
			BundleGlob: cfg.remoteBundleGlob(),
		},
	}
	setRunnerTestDepsArgs(cfg, &args)

	if err := os.MkdirAll(cfg.remoteOutDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %v", err)
	}
	// At the end of tests remoteOutDir should be empty. Otherwise os.Remove
	// fails and the directory is left for debugging.
	defer os.Remove(cfg.remoteOutDir)

	// Backfill deprecated fields in case we're executing an old test runner.
	args.FillDeprecated()

	cmd := remoteRunnerCommand(ctx, cfg)
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

	crf := func(testName, dst string) error {
		src := filepath.Join(args.RunTests.BundleArgs.OutDir, testName)
		return os.Rename(src, dst)
	}
	results, _, rerr := readTestOutput(ctx, cfg, stdout, crf, nil)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil {
		return results, stderrReader.appendToError(err, stderrTimeout)
	}

	cfg.Logger.Logf("Ran %v remote test(s) in %v", len(results), time.Now().Sub(start).Round(time.Millisecond))

	return results, rerr
}
