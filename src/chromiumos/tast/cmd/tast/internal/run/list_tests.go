// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"

	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/ssh"
)

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *Config, state *State) ([]*EntityResult, error) {
	testsToRun, testsToSkip, _, err := findTestsForShard(ctx, cfg, state)
	if err != nil {
		return nil, err
	}
	if cfg.shardIndex == 0 {
		testsToRun = append(testsToRun, testsToSkip...)
	}
	return testsToRun, nil
}

// listAllTests returns the whole tests whether they will be skipped or not..
func listAllTests(ctx context.Context, cfg *Config, state *State) ([]*EntityResult, error) {
	var tests []testing.EntityWithRunnabilityInfo
	if cfg.runLocal {
		hst, err := connectToTarget(ctx, cfg, state)
		if err != nil {
			return nil, err
		}
		localTests, err := listLocalTests(ctx, cfg, state, hst)
		if err != nil {
			return nil, err
		}
		tests = append(tests, localTests...)
	}
	if cfg.runRemote {
		remoteTests, err := listRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, err
		}
		tests = append(tests, remoteTests...)
	}

	results := make([]*EntityResult, len(tests))
	for i := 0; i < len(tests); i++ {
		results[i] = &EntityResult{EntityInfo: tests[i].EntityInfo, SkipReason: tests[i].SkipReason}
	}
	return results, nil
}

// listLocalTests returns a list of local tests to run.
func listLocalTests(ctx context.Context, cfg *Config, state *State, hst *ssh.Conn) ([]testing.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		localRunnerCommand(ctx, cfg, hst), cfg, state, cfg.localBundleGlob())
}

// listRemoteTests returns a list of remote tests to run.
func listRemoteTests(ctx context.Context, cfg *Config, state *State) ([]testing.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		remoteRunnerCommand(ctx, cfg), cfg, state, cfg.remoteBundleGlob())
}

func runListTestsCommand(r runnerCmd, cfg *Config, state *State, glob string) ([]testing.EntityWithRunnabilityInfo, error) {
	var ts []testing.EntityWithRunnabilityInfo
	args := &runner.Args{
		Mode: runner.ListTestsMode,
		ListTests: &runner.ListTestsArgs{
			BundleArgs: bundle.ListTestsArgs{
				FeatureArgs: *featureArgsFromConfig(cfg, state),
				Patterns:    cfg.Patterns,
			},
			BundleGlob: glob,
		},
	}
	if err := runTestRunnerCommand(
		r,
		args,
		&ts,
	); err != nil {
		return nil, err
	}
	return ts, nil
}
