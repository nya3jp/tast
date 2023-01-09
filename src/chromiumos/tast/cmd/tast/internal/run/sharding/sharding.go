// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sharding implements the test sharding algorithm.
package sharding

import (
	"crypto/sha256"
	"math/big"

	"chromiumos/tast/cmd/tast/internal/run/driver"
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
	var startIdx, endIdx, shardSpan int

	if len(tests)%totalShards > 0 {
		// If # of tests is not evenly divisible by totalShards, then
		// shard span is rounded up.
		// [Example] Tests: A,B,C,D,E,F,G  || totalShards=3 -> then shardSpan=3.
		// Shard0=A,B,C; Shard1=D,E,F; Shard2=G
		shardSpan = (len(tests) / totalShards) + 1
	} else {
		shardSpan = (len(tests) / totalShards)
	}

	startIdx = shardIndex * shardSpan
	if shardIndex+1 == totalShards {
		// This is the last index
		endIdx = len(tests) - 1
	} else {
		endIdx = (shardIndex+1)*shardSpan - 1
	}

	var includes, excludes []*driver.BundleEntity
	for i, test := range tests {
		if i >= startIdx && i <= endIdx {
			includes = append(includes, test)
		} else {
			excludes = append(excludes, test)
		}
	}

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
