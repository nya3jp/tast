// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"fmt"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testing"
)

func TestFindPatternsForShard(t *gotesting.T) {
	tests := []*testing.TestInstance{
		{Name: "pkg.Test0", Desc: "This is test 0"},
		{Name: "pkg.Test1", Desc: "This is test 1"},
		{Name: "pkg.Test2", Desc: "This is test 2"},
		{Name: "pkg.Test3", Desc: "This is test 3"},
		{Name: "pkg.Test4", Desc: "This is test 4"},
		{Name: "pkg.Test5", Desc: "This is test 5"},
		{Name: "pkg.Test6", Desc: "This is test 6"},
	}

	reg := testing.NewRegistry()
	for _, t := range tests {
		reg.AddTestInstance(t)
	}

	env := runtest.SetUp(t, runtest.WithLocalBundle(reg), runtest.WithRemoteBundle(testing.NewRegistry()))
	cfg := env.Config()
	cfg.TotalShards = 3
	state := env.State()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(context.Background())

	processed := make(map[string]bool)
	for shardIndex := 0; shardIndex < cfg.TotalShards; shardIndex++ {
		cfg.ShardIndex = shardIndex
		testsToRun, testsToSkip, testsNotInShard, err := FindTestsForShard(context.Background(), cfg, state, cc)
		if err != nil {
			t.Fatal("Failed to find tests for shard: ", err)
		}
		if len(testsToRun)+len(testsNotInShard)+len(testsToSkip) != len(tests) {
			t.Fatalf("The sum of numbers of tests in the shard (%v), outside the shard (%v) and skipped tests(%v) does not match the number of tests (%v)",
				len(testsToRun), len(testsNotInShard), len(testsToSkip), len(tests))
		}
		for _, tr := range testsToRun {
			name := tr.Name
			if processed[name] {
				t.Fatalf("Test %q is in more than one shard", name)
			}
			processed[name] = true
		}
	}
	if len(processed) != len(tests) {
		t.Fatal("Some tests are missing")
	}
}

// testFindShardIndices tests whether the function findShardIndices returning the correct indices.
func testFindShardIndices(numTests, totalShards, shardIndex, wantedStartIndex, wantedEndIndex int) (err error) {
	startIndex, endIndex := findShardIndices(numTests, totalShards, shardIndex)
	if startIndex != wantedStartIndex || endIndex != wantedEndIndex {
		return fmt.Errorf("findShardIndices(%v, %v, %v)=(%v, %v); want (%v, %v)",
			numTests, totalShards, shardIndex, startIndex, endIndex, wantedStartIndex, wantedEndIndex)
	}
	return nil
}

// TestFindShardIndices makes sure findShardIndices return expected indices.
func TestFindShardIndices(t *gotesting.T) {
	t.Parallel()
	tests := []struct {
		purpose                                                               string
		totalTests, totalShards, shardIndex, wantedStartIndex, wantedEndIndex int
	}{
		{"the last shard of an evenly distributed shards", 9, 3, 0, 0, 3},
		{"the middle shard of an evenly distributed shards", 9, 3, 1, 3, 6},
		{"the last shard of an evenly distributed shards", 9, 3, 2, 6, 9},
		{"the first shard of an unevenly distributed shards", 11, 3, 0, 0, 4},
		{"the middle shard of an unevenly distributed shards", 11, 3, 1, 4, 8},
		{"the last shard of an unevenly distributed shards", 11, 3, 2, 8, 11},
		{"the case that there are more shards than tests and specified shard has a test", 9, 10, 0, 0, 1},
		{"the case that there are more shards than tests and specified shard has no test", 9, 10, 9, 9, 9},
		{"the case that the shard index is greater than the number of tests", 9, 12, 11, 9, 9},
	}
	for _, tt := range tests {
		if err := testFindShardIndices(tt.totalTests, tt.totalShards, tt.shardIndex, tt.wantedStartIndex, tt.wantedEndIndex); err != nil {
			t.Errorf("Failed in testing for %v: %v", tt.purpose, err)
		}
	}
}

func TestListLocalTests(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.Test",
		Desc: "This is a test",
		Attr: []string{"attr1", "attr2"},
	})
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.AnotherTest",
		Desc: "Another test",
	})

	env := runtest.SetUp(t, runtest.WithLocalBundle(reg))
	cfg := env.Config()
	state := env.State()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(context.Background())

	conn, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListLocalTests(context.Background(), cfg, state, conn.SSHConn())
	if err != nil {
		t.Fatal("Failed to list local tests: ", err)
	}

	want := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(jsonprotocol.EntityInfo{}, "Bundle")); diff != "" {
		t.Errorf("Unexpected list of local tests (-got +want):\n%v", diff)
	}
}

func TestListRemoteTests(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.Test1",
		Desc: "First description",
		Attr: []string{"attr1", "attr2"},
		Pkg:  "pkg",
	})
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg2.Test2",
		Desc: "Second description",
		Attr: []string{"attr3"},
		Pkg:  "pkg2",
	})

	env := runtest.SetUp(t, runtest.WithRemoteBundle(reg))
	cfg := env.Config()
	state := env.State()

	got, err := listRemoteTests(context.Background(), cfg, state)
	if err != nil {
		t.Error("Failed to list remote tests: ", err)
	}

	want := []*jsonprotocol.EntityWithRunnabilityInfo{
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg.Test1",
				Desc: "First description",
				Attr: []string{"attr1", "attr2"},
				Pkg:  "pkg",
			},
		},
		{
			EntityInfo: jsonprotocol.EntityInfo{
				Name: "pkg2.Test2",
				Desc: "Second description",
				Attr: []string{"attr3"},
				Pkg:  "pkg2",
			},
		},
	}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(jsonprotocol.EntityInfo{}, "Bundle")); diff != "" {
		t.Errorf("Unexpected list of remote tests (-got +want):\n%v", diff)
	}
}
