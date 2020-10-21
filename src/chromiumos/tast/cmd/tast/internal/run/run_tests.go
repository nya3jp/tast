// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"strings"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/testing"
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

	cfg.TestNames = cfg.Patterns

	testNames, testsNotInShard, err := findPatternsForShard(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test patterns for specified shard")
	}
	cfg.TestNames = testNames
	if len(cfg.Patterns) > 0 && len(cfg.TestNames) == 0 {
		// TestNames equal to nil is different from Patterns equal to nil.
		// Patterns equal to nil means all tests need to run.
		// TestsName equal to nil meant no tests will be run.
		// Return here to prevent any tests to be run.
		return testsNotInShard, nil
	}

	cfg.startedRun = true

	// return if there are no test to run.
	var results []*EntityResult
	if cfg.runLocal {
		lres, err := runLocalTests(ctx, cfg)
		results = append(results, lres...)
		if err != nil {
			// TODO(derat): While test runners are always supposed to report success even if tests fail,
			// it'd probably be better to run both types here even if one fails.
			return append(results, testsNotInShard...), err
		}
	}

	// Turn down the ephemeral devserver before running remote tests. Some remote tests
	// in the meta category run the tast command which starts yet another ephemeral devserver
	// and reverse forwarding port can conflict.
	closeEphemeralDevserver(ctx, cfg)

	// Run remote tests and merge the results.
	if !cfg.runRemote {
		return append(results, testsNotInShard...), nil
	}

	// Run remote tests and merge the results.
	rres, err := runRemoteTests(ctx, cfg)
	results = append(results, rres...)
	return append(results, testsNotInShard...), err
}

// findPatternsForShard finds the pattern for a subset of tests based on shard index.
func findPatternsForShard(ctx context.Context, cfg *Config) (testNames []string, testsNotInShard []*EntityResult, err error) {
	if cfg.totalShards <= 1 && cfg.shardIndex == 0 {
		// Need this check to prevent some unit tests from failing.
		return cfg.Patterns, nil, nil
	}
	if cfg.totalShards < 1 {
		// The number of shards should be at least one.
		cfg.totalShards = 1
	}
	if cfg.shardIndex < 0 || cfg.shardIndex >= cfg.totalShards {
		return nil, nil, errors.Errorf("shard index %v is out of range", cfg.shardIndex)
	}
	tests, testsToSkip, err := listRunnableTests(ctx, cfg)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "fails to find runnable tests for patterns %q", cfg.Patterns)
	}

	testsNotInShard = append(testsNotInShard, testsToSkip...)

	startIndex, endIndex, err := findShardIndices(len(tests), cfg.totalShards, cfg.shardIndex)
	if err != nil {
		// Since users may not know how many tests will be available to test,
		// they may specify an index that is out of range.
		// Therefore, we skip everything.
		cfg.Logger.Logf("skips all test because fails in find tests in shard: %v", err)
		startIndex = 0
		endIndex = 0
	}
	for i := startIndex; i < endIndex; i++ {
		testNames = append(testNames, tests[i].Name)
	}
	const skipReason = "test is not in the specified shard"
	for i := 0; i < startIndex; i++ {
		tests[i].SkipReason = skipReason
		testsNotInShard = append(testsNotInShard, tests[i])
	}
	for i := endIndex; i < len(tests); i++ {
		tests[i].SkipReason = skipReason
		testsNotInShard = append(testsNotInShard, tests[i])
	}
	return testNames, testsNotInShard, nil
}

// listRunnableTests finds runnable tests that fit the cfg.Patterns.
func listRunnableTests(ctx context.Context, cfg *Config) (testsToInclude, testsToSkip []*EntityResult, err error) {
	tests, err := listTests(ctx, cfg)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot list tests for patterns %q", cfg.Patterns)
	}

	for _, t := range tests {
		testInstance := testing.TestInstance{
			Name:         t.Name,
			Pkg:          t.Pkg,
			Desc:         t.Desc,
			Contacts:     t.Contacts,
			Attr:         t.Attr,
			Data:         t.Data,
			Vars:         t.Vars,
			VarDeps:      t.VarDeps,
			SoftwareDeps: t.SoftwareDeps,
			ServiceDeps:  t.ServiceDeps,
			Fixture:      t.Fixture,
			Timeout:      t.Timeout,
		}
		r := testInstance.ShouldRun(&dep.Features{
			Var:      cfg.testVars,
			Software: cfg.softwareFeatures,
			Hardware: &dep.HardwareFeatures{Features: cfg.hardwareFeatures},
		})
		if r.OK() {
			testsToInclude = append(testsToInclude, t)
		} else {
			var reasons []string
			reasons = append(reasons, r.SkipReasons...)
			reasons = append(reasons, r.Errors...)
			t.SkipReason = strings.Join(reasons, ", ")
			testsToSkip = append(testsToSkip, t)
		}
	}
	return testsToInclude, testsToSkip, nil
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
