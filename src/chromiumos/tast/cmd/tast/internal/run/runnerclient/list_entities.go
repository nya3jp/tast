// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"fmt"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/ssh"
)

// FindTestsForShard finds the pattern for a subset of tests based on shard index.
func FindTestsForShard(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (testsToRun, testsToSkip, testsNotInShard []*resultsjson.Result, err error) {
	tests, testsToSkip, err := listRunnableTests(ctx, cfg, state, cc)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "fails to find runnable tests for patterns %q", cfg.Patterns)
	}

	startIndex, endIndex := findShardIndices(len(tests), cfg.TotalShards, cfg.ShardIndex)
	testsToRun = tests[startIndex:endIndex]

	const skipReason = "test is not in the specified shard"
	for i := 0; i < startIndex; i++ {
		tests[i].SkipReason = skipReason
		testsNotInShard = append(testsNotInShard, tests[i])
	}
	for i := endIndex; i < len(tests); i++ {
		tests[i].SkipReason = skipReason
		testsNotInShard = append(testsNotInShard, tests[i])
	}
	return testsToRun, testsToSkip, testsNotInShard, nil
}

// listRunnableTests finds runnable tests that fit the cfg.Patterns.
func listRunnableTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (testsToInclude, testsToSkip []*resultsjson.Result, err error) {
	tests, err := listAllTests(ctx, cfg, state, cc)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot list tests for patterns %q", cfg.Patterns)
	}
	for _, t := range tests {
		if t.SkipReason == "" {
			testsToInclude = append(testsToInclude, t)
		} else {
			testsToSkip = append(testsToSkip, t)
		}
	}
	return testsToInclude, testsToSkip, nil
}

// findShardIndices find the start and end index of a shard.
func findShardIndices(numTests, totalShards, shardIndex int) (startIndex, endIndex int) {
	numTestsPerShard := numTests / totalShards
	extraTests := numTests % totalShards

	// The number of tests would be different for different shard index.
	if shardIndex < extraTests {
		// First few shards will have one extra test.
		numTestsPerShard++
		startIndex = shardIndex * numTestsPerShard
	} else {
		startIndex = shardIndex*numTestsPerShard + extraTests
	}

	endIndex = startIndex + numTestsPerShard
	return startIndex, endIndex
}

// listAllTests returns the whole tests whether they will be skipped or not..
func listAllTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) ([]*resultsjson.Result, error) {
	var tests []*resultsjson.Result
	if cfg.RunLocal {
		conn, err := cc.Conn(ctx)
		if err != nil {
			return nil, err
		}
		ts, err := ListLocalTests(ctx, cfg, state, conn.SSHConn())
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			tests = append(tests, &resultsjson.Result{
				Test:       *resultsjson.NewTest(&t.EntityInfo),
				SkipReason: t.SkipReason,
				BundleType: resultsjson.LocalBundle,
			})
		}
	}
	if cfg.RunRemote {
		ts, err := listRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			tests = append(tests, &resultsjson.Result{
				Test:       *resultsjson.NewTest(&t.EntityInfo),
				SkipReason: t.SkipReason,
				BundleType: resultsjson.RemoteBundle,
			})
		}
	}
	return tests, nil
}

// ListLocalTests returns a list of local tests to run.
func ListLocalTests(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) ([]*jsonprotocol.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		localRunnerCommand(ctx, cfg, hst), cfg, state, cfg.LocalBundleGlob())
}

// ListLocalFixtures returns a map from bundle to fixtures.
func ListLocalFixtures(ctx context.Context, cfg *config.Config, hst *ssh.Conn) (map[string][]*jsonprotocol.EntityInfo, error) {
	var localFixts jsonprotocol.RunnerListFixturesResult
	if err := runTestRunnerCommand(
		localRunnerCommand(ctx, cfg, hst), &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerListFixturesMode,
			ListFixtures: &jsonprotocol.RunnerListFixturesArgs{
				BundleGlob: cfg.LocalBundleGlob(),
			},
		}, &localFixts); err != nil {
		return nil, fmt.Errorf("listing local fixtures: %v", err)
	}
	return localFixts.Fixtures, nil
}

// listRemoteTests returns a list of remote tests to run.
func listRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) ([]*jsonprotocol.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		remoteRunnerCommand(ctx, cfg), cfg, state, cfg.RemoteBundleGlob())
}

// listRemoteFixtures returns a map from bundle to fixtures.
func listRemoteFixtures(ctx context.Context, cfg *config.Config) (map[string][]*jsonprotocol.EntityInfo, error) {
	var remoteFixts jsonprotocol.RunnerListFixturesResult
	if err := runTestRunnerCommand(
		remoteRunnerCommand(ctx, cfg), &jsonprotocol.RunnerArgs{
			Mode: jsonprotocol.RunnerListFixturesMode,
			ListFixtures: &jsonprotocol.RunnerListFixturesArgs{
				BundleGlob: cfg.RemoteBundleGlob(),
			},
		}, &remoteFixts); err != nil {
		return nil, fmt.Errorf("listing remote fixtures: %v", err)
	}
	return remoteFixts.Fixtures, nil
}

func runListTestsCommand(r runnerCmd, cfg *config.Config, state *config.State, glob string) ([]*jsonprotocol.EntityWithRunnabilityInfo, error) {
	args := &jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerListTestsMode,
		ListTests: &jsonprotocol.RunnerListTestsArgs{
			BundleArgs: jsonprotocol.BundleListTestsArgs{
				FeatureArgs: *featureArgsFromConfig(cfg, state),
				Patterns:    cfg.Patterns,
			},
			BundleGlob: glob,
		},
	}
	var res jsonprotocol.RunnerListTestsResult
	if err := runTestRunnerCommand(
		r,
		args,
		&res,
	); err != nil {
		return nil, fmt.Errorf("listing tests %v: %v", cfg.Patterns, err)
	}
	return res, nil
}
