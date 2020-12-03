// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"

	"chromiumos/tast/errors"
)

func runTests(ctx context.Context, cfg *Config) ([]*EntityResult, error) {
	if err := getDUTInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}

	if cfg.osVersion == "" {
		cfg.Logger.Log("Target version: not available from target")
	} else {
		cfg.Logger.Logf("Target version: %v", cfg.osVersion)
	}

	if err := getInitialSysInfo(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}

	testsToRun, testsToSkip, testsNotInShard, err := findTestsForShard(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test patterns for specified shard")
	}

	// We include all tests to be skipped in shard 0
	if cfg.shardIndex == 0 {
		testsToRun = append(testsToRun, testsToSkip...)
		testsToSkip = nil
	}

	cfg.testNames = nil
	for _, t := range testsToRun {
		cfg.testNames = append(cfg.testNames, t.Name)
	}
	cfg.TestNamesToSkip = nil
	for _, t := range testsToSkip {
		cfg.TestNamesToSkip = append(cfg.TestNamesToSkip, t.Name)
	}
	for _, t := range testsNotInShard {
		cfg.TestNamesToSkip = append(cfg.TestNamesToSkip, t.Name)
	}

	if cfg.totalShards > 1 {
		cfg.Logger.Logf("Running shard %d/%d (tests %d/%d)", cfg.shardIndex+1, cfg.totalShards,
			len(cfg.testNames), len(cfg.testNames)+len(cfg.TestNamesToSkip))
	}
	if len(testsToRun) == 0 {
		// No tests to run.
		return nil, nil
	}

	var results []*EntityResult
	cfg.startedRun = true

	if cfg.runLocal {
		lres, err := runLocalTests(ctx, cfg)
		results = append(results, lres...)
		if err != nil {
			// TODO(derat): While test runners are always supposed to report success even if tests fail,
			// it'd probably be better to run both types here even if one fails.
			return results, err
		}
	}

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// in the meta category run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	if !cfg.runRemote {
		return results, nil
	}

	// Run remote tests and merge the results.
	rres, err := runRemoteTests(ctx, cfg)
	results = append(results, rres...)
	return results, err
}

// findTestsForShard finds the pattern for a subset of tests based on shard index.
func findTestsForShard(ctx context.Context, cfg *Config) (testsToRun, testsToSkip, testsNotInShard []*EntityResult, err error) {
	tests, testsToSkip, err := listRunnableTests(ctx, cfg)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "fails to find runnable tests for patterns %q", cfg.Patterns)
	}

	startIndex, endIndex := findShardIndices(len(tests), cfg.totalShards, cfg.shardIndex)
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
func listRunnableTests(ctx context.Context, cfg *Config) (testsToInclude, testsToSkip []*EntityResult, err error) {
	tests, err := listTests(ctx, cfg)
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
