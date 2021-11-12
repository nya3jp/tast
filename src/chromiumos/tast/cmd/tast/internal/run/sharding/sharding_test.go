// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sharding_test

import (
	"fmt"
	gotesting "testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/sharding"
	"chromiumos/tast/internal/protocol"
)

// makeTests creates a list of BundleEntity from pattern.
// For each character in the pattern, a BundleEntity having the character as
// its name is created. If a character is lower-cased, it is marked as skipped.
func makeTests(pattern string) []*driver.BundleEntity {
	var tests []*driver.BundleEntity
	for _, ch := range pattern {
		t := &driver.BundleEntity{
			Bundle: "bundle",
			Resolved: &protocol.ResolvedEntity{
				Entity: &protocol.Entity{
					Name: string(ch),
				},
			},
		}
		if unicode.IsLower(ch) {
			t.Resolved.Skip = &protocol.Skip{Reasons: []string{"Skip it"}}
		}
		tests = append(tests, t)
	}
	return tests
}

func TestCompute(t *gotesting.T) {
	for _, tc := range []struct {
		name   string
		tests  []*driver.BundleEntity
		shards []*sharding.Shard
	}{
		{
			"single",
			makeTests("AxyBCzEF"),
			[]*sharding.Shard{
				{Included: makeTests("AxyBCzEF"), Excluded: makeTests("")},
			},
		},
		{
			"even",
			makeTests("ABCDEFGHI"),
			[]*sharding.Shard{
				{Included: makeTests("ABC"), Excluded: makeTests("DEFGHI")},
				{Included: makeTests("DEF"), Excluded: makeTests("ABCGHI")},
				{Included: makeTests("GHI"), Excluded: makeTests("ABCDEF")},
			},
		},
		{
			"uneven",
			makeTests("ABCDEFGHIJK"),
			[]*sharding.Shard{
				{Included: makeTests("ABCD"), Excluded: makeTests("EFGHIJK")},
				{Included: makeTests("EFGH"), Excluded: makeTests("ABCDIJK")},
				{Included: makeTests("IJK"), Excluded: makeTests("ABCDEFGH")},
			},
		},
		{
			"skips",
			makeTests("AxByCzD"),
			[]*sharding.Shard{
				{Included: makeTests("AxB"), Excluded: makeTests("yCzD")},
				{Included: makeTests("yCz"), Excluded: makeTests("AxBD")},
				{Included: makeTests("D"), Excluded: makeTests("AxByCz")},
			},
		},
		{
			"sparse",
			makeTests("AxB"),
			[]*sharding.Shard{
				{Included: makeTests("A"), Excluded: makeTests("xB")},
				{Included: makeTests("x"), Excluded: makeTests("AB")},
				{Included: makeTests("B"), Excluded: makeTests("Ax")},
			},
		},
		{
			"zero",
			makeTests(""),
			[]*sharding.Shard{
				{Included: makeTests(""), Excluded: makeTests("")},
				{Included: makeTests(""), Excluded: makeTests("")},
				{Included: makeTests(""), Excluded: makeTests("")},
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			for index, want := range tc.shards {
				t.Run(fmt.Sprintf("shard%d", index), func(t *gotesting.T) {
					got := sharding.Compute(tc.tests, index, len(tc.shards))
					if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
						t.Errorf("Mismatch (-got +want):\n%s", diff)
					}
				})
			}
		})
	}
}
