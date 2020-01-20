// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"chromiumos/tast/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/timing"
)

// runRemoteTests runs the remote test runner and reads its output.
func runRemoteTests(ctx context.Context, cfg *Config) ([]TestResult, error) {
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.remoteOutDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %v", err)
	}
	// At the end of tests remoteOutDir should be empty. Otherwise os.Remove
	// fails and the directory is left for debugging.
	defer os.Remove(cfg.remoteOutDir)

	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				Patterns:       cfg.Patterns,
				DataDir:        cfg.remoteDataDir,
				OutDir:         cfg.remoteOutDir,
				TestVars:       cfg.testVars,
				Target:         cfg.Target,
				KeyFile:        cfg.KeyFile,
				KeyDir:         cfg.KeyDir,
				TastPath:       exe,
				LocalBundleDir: cfg.localBundleDir,
				RunFlags: []string{
					"-keyfile=" + cfg.KeyFile,
					"-keydir=" + cfg.KeyDir,
					"-remoterunner=" + cfg.remoteRunner,
					"-remotebundledir=" + cfg.remoteBundleDir,
					"-remotedatadir=" + cfg.remoteDataDir,
				},
				HeartbeatInterval: heartbeatInterval,
			},
			BundleGlob: cfg.remoteBundleGlob(),
		},
	}
	setRunnerTestDepsArgs(cfg, &args)

	r := newRemoteRunner(ctx, cfg)
	cfg.Logger.Logf("Starting %v locally", r.Path)
	results, _, err := runTestsOnce(
		ctx,
		cfg,
		r,
		&args,
		func(testName, dst string) error {
			src := filepath.Join(args.RunTests.BundleArgs.OutDir, testName)
			return os.Rename(src, dst)
		},
		nil)
	return results, err
}
