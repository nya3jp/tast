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

	testNames, testNamesNotInShard, err := findPatternsForShard(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test patterns for specified shard")
	}
	cfg.Patterns = testNames
	cfg.startedRun = true

	if len(testNamesNotInShard) > 0 {
		cfg.Logger.Log("Following tests will not be run due to sharding")
		for _, t := range testNamesNotInShard {
			cfg.Logger.Log("        ", t)
		}
	}

	var results []*EntityResult
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

	// Run remote tests and merge the results.
	if !cfg.runRemote {
		return results, nil
	}

	rres, err := runRemoteTests(ctx, cfg)
	results = append(results, rres...)
	return results, err
}

// findPatternsForShard finds the pattern for a subset of tests based on shard index.
func findPatternsForShard(ctx context.Context, cfg *Config) (testNames, testNamesNotInShard []string, err error) {
	if cfg.totalShards < 1 {
		return cfg.Patterns, nil, nil
	}
	if cfg.shardIndex < 0 || cfg.shardIndex >= cfg.totalShards {
		return cfg.Patterns, nil, errors.Errorf("shard index %v is out of range", cfg.shardIndex)
	}
	tests, err := listTests(ctx, cfg)
	if err != nil {
		return cfg.Patterns, nil, errors.Wrapf(err, "cannot get tests for patterns %q", cfg.Patterns)
	}
	startIndex, endIndex, err := findShardIndices(len(tests), cfg.totalShards, cfg.shardIndex)
	if err != nil {
		return cfg.Patterns, nil, errors.Wrap(err, "fail to get test indices for the shard")
	}
	for i := startIndex; i < endIndex; i++ {
		testNames = append(testNames, tests[i].Name)
	}
	for i := 0; i < startIndex; i++ {
		testNamesNotInShard = append(testNamesNotInShard, tests[i].Name)
	}
	for i := endIndex; i < len(tests); i++ {
		testNamesNotInShard = append(testNamesNotInShard, tests[i].Name)
	}
	return testNames, testNamesNotInShard, nil
}

// findShardIndices find the start and end index of a shard.
func findShardIndices(numTests, totalShards, shardIndex int) (startIndex, endIndex int, err error) {
	if totalShards < 2 && shardIndex == 0 {
		return 0, numTests, nil
	}
	if totalShards < 1 {
		totalShards = 1
	}
	if numTests <= totalShards {
		if shardIndex < numTests {
			return shardIndex, shardIndex + 1, nil
		}
		return 0, numTests, errors.Errorf("invalid shard index %v where number of shards is %v and number of tests is %v",
			shardIndex, totalShards, numTests)
	}
	numTestsPerShard := numTests / totalShards
	extraTests := numTests % totalShards
	startIndex = shardIndex * numTestsPerShard

	if extraTests > 0 {
		// The number of tests would be different for different shard index.
		if shardIndex < extraTests {
			// First few shards will have one extra test.
			numTestsPerShard = numTestsPerShard + 1
			startIndex = shardIndex * numTestsPerShard
		} else {
			startIndex = startIndex + extraTests
		}
	}
	if shardIndex < 0 || startIndex >= numTests {
		return 0, numTests, errors.Errorf("invalid shard index %v where number of shards is %v and number of tests is %v",
			shardIndex, totalShards, numTests)
	}
	endIndex = startIndex + numTestsPerShard
	if endIndex > numTests {
		endIndex = numTests
	}
	return startIndex, endIndex, nil
}
