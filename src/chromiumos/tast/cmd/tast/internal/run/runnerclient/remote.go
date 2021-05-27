// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/timing"
)

// RunRemoteTests runs the remote test runner and reads its output.
func RunRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) ([]*resultsjson.Result, error) {
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	if err := os.MkdirAll(cfg.RemoteOutDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %v", err)
	}
	// At the end of tests RemoteOutDir should be empty. Otherwise os.Remove
	// fails and the directory is left for debugging.
	defer os.Remove(cfg.RemoteOutDir)

	runTests := func(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
		return runRemoteTestsOnce(ctx, cfg, state, patterns)
	}
	beforeRetry := func(ctx context.Context) bool { return true }

	start := time.Now()
	names := make([]string, len(state.TestsToRun), len(state.TestsToRun))
	// TODO(crbug/1190653): Filter out local tests. Currently simply removing
	// local tests doesn't work, because if resulting names become empty, it
	// instructs to run all the tests.
	for i, t := range state.TestsToRun {
		names[i] = t.Name
	}
	results, err := runTestsWithRetry(ctx, cfg, names, runTests, beforeRetry)
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
func runRemoteTestsOnce(ctx context.Context, cfg *config.Config, state *config.State, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
	ctx, st := timing.Start(ctx, "run_remote_tests_once")
	defer st.End()

	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}

	buildArtifactsURL := cfg.BuildArtifactsURL
	if buildArtifactsURL == "" {
		buildArtifactsURL = state.DUTInfo.GetDefaultBuildArtifactsUrl()
	}

	runFlags := []string{
		"-build=" + strconv.FormatBool(cfg.Build),
		"-keyfile=" + cfg.KeyFile,
		"-keydir=" + cfg.KeyDir,
		"-remoterunner=" + cfg.RemoteRunner,
		"-remotebundledir=" + cfg.RemoteBundleDir,
		"-remotedatadir=" + cfg.RemoteDataDir,
		"-localrunner=" + cfg.LocalRunner,
		"-localbundledir=" + cfg.LocalBundleDir,
		"-localdatadir=" + cfg.LocalDataDir,
		"-devservers=" + strings.Join(cfg.Devservers, ","),
		"-buildartifactsurl=" + buildArtifactsURL,
	}
	for role, dut := range cfg.CompanionDUTs {
		runFlags = append(runFlags, fmt.Sprintf("-companiondut=%s:%s", role, dut))
	}
	args := jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerRunTestsMode,
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleArgs: jsonprotocol.BundleRunTestsArgs{
				FeatureArgs:       *featureArgsFromConfig(cfg, state),
				Patterns:          patterns,
				DataDir:           cfg.RemoteDataDir,
				OutDir:            cfg.RemoteOutDir,
				Target:            cfg.Target,
				KeyFile:           cfg.KeyFile,
				KeyDir:            cfg.KeyDir,
				TastPath:          exe,
				RunFlags:          runFlags,
				LocalBundleDir:    cfg.LocalBundleDir,
				Devservers:        state.RemoteDevservers,
				TLWServer:         cfg.TLWServer,
				DUTName:           cfg.Target,
				HeartbeatInterval: heartbeatInterval,
				DownloadMode:      cfg.DownloadMode,
				BuildArtifactsURL: buildArtifactsURL,
				CompanionDUTs:     cfg.CompanionDUTs,
			},
			BundleGlob: cfg.RemoteBundleGlob(),
		},
	}

	// Backfill deprecated fields in case we're executing an old test runner.
	args.FillDeprecated()

	cmd := remoteRunnerCommand(cfg)
	cfg.Logger.Logf("Starting %v locally", cfg.RemoteRunner)
	proc, err := cmd.Interact(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	// TODO(b/187793617): Fix leak of proc.

	if err := json.NewEncoder(proc.Stdin()).Encode(&args); err != nil {
		return nil, nil, fmt.Errorf("write to stdin: %v", err)
	}
	if err := proc.Stdin().Close(); err != nil {
		return nil, nil, fmt.Errorf("close stdin: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results, unstarted, rerr := readTestOutput(ctx, cfg, state, proc.Stdout(), os.Rename, nil)
	canceled := false
	if errors.Is(rerr, ErrTerminate) {
		canceled = true
		cancel()
	}

	stderrReader := newFirstLineReader(proc.Stderr())

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := proc.Wait(ctx); err != nil && !canceled {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}
