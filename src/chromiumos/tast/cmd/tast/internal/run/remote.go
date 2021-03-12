// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/devserver"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/timing"
)

// startEphemeralDevserverForRemoteTests starts an ephemeral devserver for remote tests.
func startEphemeralDevserverForRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) (*devserver.Ephemeral, error) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen to a local port: %v", err)
	}

	cacheDir := filepath.Join(cfg.TastDir, "devserver", "static")
	es, err := devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets)
	if err != nil {
		return nil, err
	}

	state.RemoteDevservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	cfg.Logger.Log("Starting ephemeral devserver at ", state.RemoteDevservers[0], " for remote tests")
	return es, nil
}

// runRemoteTests runs the remote test runner and reads its output.
func runRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) ([]*jsonprotocol.EntityResult, error) {
	ctx, st := timing.Start(ctx, "run_remote_tests")
	defer st.End()

	if err := os.MkdirAll(cfg.RemoteOutDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %v", err)
	}
	// At the end of tests RemoteOutDir should be empty. Otherwise os.Remove
	// fails and the directory is left for debugging.
	defer os.Remove(cfg.RemoteOutDir)

	runTests := func(ctx context.Context, patterns []string) (results []*jsonprotocol.EntityResult, unstarted []string, err error) {
		return runRemoteTestsOnce(ctx, cfg, state, patterns)
	}
	beforeRetry := func(ctx context.Context) bool { return true }

	start := time.Now()
	names := make([]string, len(cfg.TestsToRun), len(cfg.TestsToRun))
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
func runRemoteTestsOnce(ctx context.Context, cfg *config.Config, state *config.State, patterns []string) (results []*jsonprotocol.EntityResult, unstarted []string, err error) {
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

	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				FeatureArgs: *featureArgsFromConfig(cfg, state),
				Patterns:    patterns,
				DataDir:     cfg.RemoteDataDir,
				OutDir:      cfg.RemoteOutDir,
				Target:      cfg.Target,
				KeyFile:     cfg.KeyFile,
				KeyDir:      cfg.KeyDir,
				TastPath:    exe,
				RunFlags: []string{
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
				},
				LocalBundleDir:    cfg.LocalBundleDir,
				Devservers:        state.RemoteDevservers,
				TLWServer:         cfg.TLWServer,
				DUTName:           cfg.Target,
				HeartbeatInterval: heartbeatInterval,
				DownloadMode:      cfg.DownloadMode,
				BuildArtifactsURL: buildArtifactsURL,
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
