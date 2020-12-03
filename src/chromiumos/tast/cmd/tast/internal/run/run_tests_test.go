// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	internal_testing "chromiumos/tast/internal/testing"
)

func TestRunTestsFailureBeforeRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the runner always fail, and ask to check test deps so we'll get a failure before trying
	// to run tests. local() shouldn't set startedRun to true since we failed before then.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }
	td.cfg.checkTestDeps = true
	if _, err := runTests(context.Background(), &td.cfg); err == nil {
		t.Errorf("runTests unexpectedly passed")
	} else if td.cfg.startedRun {
		t.Error("runTests incorrectly reported that run was started after early failure")
	}
}

func TestRunTestsGetDUTInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.GetDUTInfoMode:
			// Just check that getDUTInfo is called; details of args are
			// tested in deps_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetDUTInfoResult{
				SoftwareFeatures: &dep.SoftwareFeatures{
					Available: []string{"foo"}, // must report non-empty features
				},
				OSVersion: osVersion,
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.checkTestDeps = true

	if _, err := runTests(context.Background(), &td.cfg); err != nil {
		t.Error("runTests failed: ", err)
	}

	expectedOSVersion := "Target version: " + osVersion
	if !strings.Contains(td.logbuf.String(), expectedOSVersion) {
		t.Errorf("Cannot find %q in log buffer %v", expectedOSVersion, td.logbuf.String())
	}
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}

func TestRunTestsGetInitialSysInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.GetSysInfoStateMode:
			// Just check that getInitialSysInfo is called; details of args are
			// tested in sys_info_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetSysInfoStateResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.collectSysInfo = true

	if _, err := runTests(context.Background(), &td.cfg); err != nil {
		t.Error("runTests failed: ", err)
	}
	if !called {
		t.Errorf("runTests did not call getInitialSysInfo")
	}
}

// TestRunTestsSkipTests check if runTests skipping testings correctly.
func TestRunTestsSkipTests(t *gotesting.T) {
	tests := []internal_testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: internal_testing.EntityInfo{
				Name:         "pkg.Test0",
				Desc:         "This is test 0",
				SoftwareDeps: []string{"has_dep"},
			},
			SkipReason: "dependency not available",
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test1", Desc: "This is test 1"},
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test2", Desc: "This is test 2"},
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test3", Desc: "This is test 3"},
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test4", Desc: "This is test 4"},
		},
		{
			EntityInfo: internal_testing.EntityInfo{
				Name:         "pkg.Test5",
				Desc:         "This is test 5",
				SoftwareDeps: []string{"has_dep"},
			},
			SkipReason: "dependency not available",
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test6", Desc: "This is test 6"},
		},
	}

	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.GetDUTInfoMode:
			// Just check that getDUTInfo is called; details of args are
			// tested in deps_test.go.
			json.NewEncoder(stdout).Encode(&runner.GetDUTInfoResult{
				SoftwareFeatures: &dep.SoftwareFeatures{
					Available: []string{"a_feature"},
				},
			})
		case runner.ListTestsMode:
			json.NewEncoder(stdout).Encode(tests)
		case runner.RunTestsMode:
			testNames := args.RunTests.BundleArgs.Patterns
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: len(testNames)})
			var count int64
			count = 2
			for _, t := range testNames {
				mw.WriteMessage(&control.EntityStart{Time: time.Unix(count, 0), Info: testing.EntityInfo{Name: t}})
				count = count + 1
				mw.WriteMessage(&control.EntityEnd{Time: time.Unix(count, 0), Name: t})
				count = count + 1
			}
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(count, 0)})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	// List matching tests instead of running them.
	td.cfg.localDataDir = "/tmp/data"
	td.cfg.Patterns = []string{"*Test*"}
	td.cfg.runLocal = true
	td.cfg.totalShards = 2
	td.cfg.checkTestDeps = true

	expectedNumPassed := 5
	expectedNumSkipped := len(tests)*2 - 5
	numPassed := 0
	numSkipped := 0
	for shardIndex := 0; shardIndex < td.cfg.totalShards; shardIndex++ {
		td.cfg.softwareFeatures = nil
		td.cfg.shardIndex = shardIndex
		testResults, err := runTests(context.Background(), &td.cfg)
		if err != nil {
			t.Fatal("Failed to run tests: ", err)
		}
		if len(testResults) != len(tests) {
			t.Fatalf("runTests returned %d results; want %d", len(testResults), len(tests))
		}
		for _, t := range testResults {
			if t.SkipReason == "" {
				numPassed = numPassed + 1
			} else {
				numSkipped = numSkipped + 1
			}
		}
	}
	if numPassed != expectedNumPassed {
		t.Fatalf("runTests returned %d passed tests; want %d", numPassed, expectedNumPassed)
	}
	if numSkipped != expectedNumSkipped {
		t.Fatalf("runTests returned %d skipped tests; want %d", numSkipped, expectedNumSkipped)
	}
}

func TestFindPatternsForShard(t *gotesting.T) {
	tests := []internal_testing.EntityInfo{
		{Name: "pkg.Test0", Desc: "This is test 0"},
		{Name: "pkg.Test1", Desc: "This is test 1"},
		{Name: "pkg.Test2", Desc: "This is test 2"},
		{Name: "pkg.Test3", Desc: "This is test 3"},
		{Name: "pkg.Test4", Desc: "This is test 4"},
		{Name: "pkg.Test5", Desc: "This is test 5"},
		{Name: "pkg.Test6", Desc: "This is test 6"},
	}
	// Make the runner print serialized tests.
	b, err := json.Marshal(&tests)
	if err != nil {
		t.Fatal(err)
	}
	td := newRemoteTestData(t, string(b), "", 0)
	defer td.close()

	// List matching tests instead of running them.
	td.cfg.remoteDataDir = "/tmp/data"
	td.cfg.Patterns = []string{"*Test*"}
	td.cfg.runRemote = true
	td.cfg.totalShards = 3
	result := make(map[string]int)
	for shardIndex := 0; shardIndex < td.cfg.totalShards; shardIndex++ {
		td.cfg.shardIndex = shardIndex
		testNames, testsNotInShard, err := findPatternsForShard(context.Background(), &td.cfg)
		if err != nil {
			t.Fatal("Failed to find patterns for shard: ", err)
		}
		if len(testNames)+len(testsNotInShard) != len(tests) {
			t.Fatalf("The sum of numbers of tests in the shard (%v) and not in the shard (%v) does not match the number of tests (%v)",
				len(testNames), len(testsNotInShard), len(tests))
		}
		for _, name := range testNames {
			result[name] = result[name] + 1
			if result[name] > 1 {
				t.Fatalf("Test %q is in more than one shard", name)
			}
		}
	}
	if len(result) != len(tests) {
		t.Fatal("Some tests are missing")
	}
}

// TestFindShardIndicesFirstEvenShard makes sure findShardIndices return correct indices for
// the first shard of an evenly distributed shards.
func TestFindShardIndicesFirstEvenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 9, 3, 0, 0, 3); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the first shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMiddleEvenShard makes sure findShardIndices return correct indices for
// the middle shard of an evenly distributed shards.
func TestFindShardIndicesMiddleEvenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 9, 3, 1, 3, 6); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the middle shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesLastEvenShard makes sure findShardIndices return correct indices for
// the last shard of an evenly distributed shards.
func TestFindShardIndicesLastEvenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 9, 3, 2, 6, 9); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the last shard of an evenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesFirstUnevenShard makes sure findShardIndices return correct indices for
// the first shard of an unevenly distributed shards.
func TestFindShardIndicesFirstUnevenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 11, 3, 0, 0, 4); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the first shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMiddleUnevenShard makes sure findShardIndices return correct indices for
// the middle shard of an unevenly distributed shards.
func TestFindShardIndicesMiddleUnevenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 11, 3, 1, 4, 8); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the middle shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesLastUnevenShard makes sure findShardIndices return correct indices for
// the last shard of an unevenly distributed shards.
func TestFindShardIndicesLastUnevenShard(t *gotesting.T) {
	if err := testFindShardIndices(t, 11, 3, 2, 8, 11); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices for the last shard of an unevenly distributed shards: %v", err)
	}
}

// TestFindShardIndicesMoreShardsThanTests makes sure findShardIndices return correct indices when
// the number of shards is greater than number of tests.
func TestFindShardIndicesMoreShardsThanTests(t *gotesting.T) {
	if err := testFindShardIndices(t, 9, 10, 0, 0, 1); err != nil {
		t.Errorf("Failed to get correct indices from findShardIndices when the number of shards is greater than number of tests: %v", err)
	}
}

// TestFindShardIndicesInvalidIndex makes sure findShardIndices return error when
// the shard index is out of range.
func TestFindShardIndicesInvalidIndex(t *gotesting.T) {
	if err := testFindShardIndices(t, 9, 3, 4, 0, 0); err != nil {
		t.Errorf("Failed to get empty slice findShardIndices when the shard index is out of range: %v", err)
	}
	if err := testFindShardIndices(t, 9, 10, 11, 0, 0); err != nil {
		t.Errorf("Failed to get empty from findShardIndices when the shard index is out of range: %v", err)
	}
}

// testFindShardIndices tests whether the function findShardIndices returning the correct indices.
func testFindShardIndices(t *gotesting.T,
	numTests, totalShards, shardIndex, wantedStartIndex, wantedEndIndex int) (err error) {
	startIndex, endIndex := findShardIndices(numTests, totalShards, shardIndex)
	if startIndex != wantedStartIndex {
		return fmt.Errorf("findShardIndices returned start index %d results; want %d", startIndex, wantedStartIndex)
	}
	if endIndex != wantedEndIndex {
		return fmt.Errorf("findShardIndices returned end index %d results; want %d", endIndex, wantedEndIndex)
	}
	return nil
}
