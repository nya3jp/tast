// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	names := make([]string, len(cfg.TestsToRun), len(cfg.TestsToRun))
	// TODO(crbug/1190653): Filter out local tests. Currently simply removing
	// local tests doesn't work, because if resulting names become empty, it
	// instructs to run all the tests.
	for i, t := range cfg.TestsToRun {
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
		buildArtifactsURL = state.DefaultBuildArtifactsURL
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results, unstarted, rerr := readTestOutput(ctx, cfg, state, stdout, os.Rename, nil)
	canceled := false
	if errors.Is(rerr, ErrTerminate) {
		canceled = true
		cancel()
	}

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	if err := cmd.Wait(); err != nil && !canceled {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}