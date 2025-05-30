// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sharding implements the test sharding algorithm.
package sharding

import (
	"crypto/sha256"
	"math/big"

	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
)

// Shard represents a set of tests included/excluded in a shard.
type Shard struct {
	// Included is a list of tests in the shard and to be requested to run.
	// This may include tests that will be skipped due to unsatisfied
	// dependencies.
	Included []*driver.BundleEntity

	// Excluded is a list of tests not in the shard and to be ignored.
	Excluded []*driver.BundleEntity
}

// ComputeAlpha computes a set of tests to include/exclude in the specified shard by lexiographic.
func ComputeAlpha(tests []*driver.BundleEntity, shardIndex, totalShards int) *Shard {
	shardSpan := len(tests) / totalShards
	remaining := len(tests) % totalShards

	// For remaining, we put 1 additional items from the beginning for
	// the first remaining shards, rather than +1 for all shards.
	// For example, if we have 9 tests: [A, B, C, D, E, F, G, H, I], and
	// totalShards to 4. We should have 3 tests at first shard, and 2
	// tests at all following 3 shards (3+2+2+2), instead of 3 tests at
	// all shards (ends up with 3+3+3+0)
	startIdx := shardIndex*shardSpan + min(shardIndex, remaining)
	endIdx := startIdx + shardSpan
	if shardIndex < remaining {
		endIdx++
	}

	var includes, excludes []*driver.BundleEntity
	includes = append(includes, tests[startIdx:endIdx]...)
	excludes = append(
		append(excludes, tests[:startIdx]...),
		tests[endIdx:]...)

	return &Shard{Included: includes, Excluded: excludes}
}

// ComputeHash computes a set of tests to include/exclude in the specified shard by hash.
// This is experimental for now, and will potentially produce empty shards. If you see
// issues when this is rolling out please file a buganizer issue to component 1152900.
func ComputeHash(tests []*driver.BundleEntity, shardIndex, totalShards int) *Shard {
	var includes, excludes []*driver.BundleEntity
	for _, test := range tests {
		// Compute sha256(Entity.Name) % totalShards.
		sum := sha256.Sum256([]byte(test.Resolved.Entity.Name))
		shaInt := new(big.Int)
		shaInt.SetBytes(sum[:])
		shaInt.Mod(shaInt, big.NewInt(int64(totalShards)))

		// If the shardIndex == the module we're in the group.
		if int(shaInt.Int64()) == shardIndex {
			includes = append(includes, test)
		} else {
			excludes = append(excludes, test)
		}
	}

	return &Shard{Included: includes, Excluded: excludes}
}
