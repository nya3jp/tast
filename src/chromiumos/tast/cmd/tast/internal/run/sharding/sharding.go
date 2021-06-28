// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sharding implements the test sharding algorithm.
package sharding

import (
	"chromiumos/tast/internal/protocol"
)

// Shard represents a set of tests included/excluded in a shard.
type Shard struct {
	// Included is a list of tests in the shard and to be requested to run.
	// This may include tests that will be skipped due to unsatisfied
	// dependencies.
	Included []*protocol.ResolvedEntity

	// Excluded is a list of tests not in the shard and to be ignored.
	Excluded []*protocol.ResolvedEntity
}

// Compute computes a set of tests to include/exclude in the specified shard.
func Compute(tests []*protocol.ResolvedEntity, shardIndex, totalShards int) *Shard {
	var runs, skips []*protocol.ResolvedEntity
	for _, t := range tests {
		if len(t.GetSkip().GetReasons()) == 0 {
			runs = append(runs, t)
		} else {
			skips = append(skips, t)
		}
	}

	startIndex, endIndex := shardIndices(len(runs), shardIndex, totalShards)

	var includes, excludes []*protocol.ResolvedEntity
	// Shard 0 contains all skipped tests.
	if shardIndex == 0 {
		includes = skips
	} else {
		excludes = skips
	}
	includes = append(includes, runs[startIndex:endIndex]...)
	excludes = append(append(excludes, runs[:startIndex]...), runs[endIndex:]...)
	return &Shard{Included: includes, Excluded: excludes}
}

func shardIndices(numTests, shardIndex, totalShards int) (startIndex, endIndex int) {
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
