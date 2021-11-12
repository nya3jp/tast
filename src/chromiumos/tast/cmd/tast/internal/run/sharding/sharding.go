// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sharding implements the test sharding algorithm.
package sharding

import (
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

// Compute computes a set of tests to include/exclude in the specified shard.
func Compute(tests []*driver.BundleEntity, shardIndex, totalShards int) *Shard {
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
