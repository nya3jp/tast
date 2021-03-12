// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/ssh"
)

// findTestsForShard finds the pattern for a subset of tests based on shard index.
func findTestsForShard(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (testsToRun, testsToSkip, testsNotInShard []*jsonprotocol.EntityResult, err error) {
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
func listRunnableTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (testsToInclude, testsToSkip []*jsonprotocol.EntityResult, err error) {
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
func listAllTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) ([]*jsonprotocol.EntityResult, error) {
	var tests []testing.EntityWithRunnabilityInfo
	if cfg.RunLocal {
		conn, err := cc.Conn(ctx)
		if err != nil {
			return nil, err
		}
		localTests, err := listLocalTests(ctx, cfg, state, conn.SSHConn())
		if err != nil {
			return nil, err
		}
		tests = append(tests, localTests...)
	}
	if cfg.RunRemote {
		remoteTests, err := listRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, err
		}
		tests = append(tests, remoteTests...)
	}

	results := make([]*jsonprotocol.EntityResult, len(tests))
	for i := 0; i < len(tests); i++ {
		results[i] = &jsonprotocol.EntityResult{EntityInfo: tests[i].EntityInfo, SkipReason: tests[i].SkipReason}
	}
	return results, nil
}

// listLocalTests returns a list of local tests to run.
func listLocalTests(ctx context.Context, cfg *config.Config, state *config.State, hst *ssh.Conn) ([]testing.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		localRunnerCommand(ctx, cfg, hst), cfg, state, cfg.LocalBundleGlob())
}

// listRemoteTests returns a list of remote tests to run.
func listRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) ([]testing.EntityWithRunnabilityInfo, error) {
	return runListTestsCommand(
		remoteRunnerCommand(ctx, cfg), cfg, state, cfg.RemoteBundleGlob())
}

func runListTestsCommand(r runnerCmd, cfg *config.Config, state *config.State, glob string) ([]testing.EntityWithRunnabilityInfo, error) {
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
