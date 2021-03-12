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

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	internal_testing "chromiumos/tast/internal/testing"
)

func TestRunTestsFailureBeforeRun(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// Make the runner always fail, and ask to check test deps so we'll get a failure before trying
	// to run tests. local() shouldn't set StartedRun to true since we failed before then.
	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }
	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	var state config.State
	if _, err := runTests(context.Background(), &td.Cfg, &state, cc); err == nil {
		t.Errorf("runTests unexpectedly passed")
	} else if state.StartedRun {
		t.Error("runTests incorrectly reported that run was started after early failure")
	}
}

func TestRunTestsGetDUTInfo(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	called := false

	osVersion := "octopus-release/R86-13312.0.2020_07_02_1108"

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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

	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Error("runTests failed: ", err)
	}

	expectedOSVersion := "Target version: " + osVersion
	if !strings.Contains(td.LogBuf.String(), expectedOSVersion) {
		t.Errorf("Cannot find %q in log buffer %v", expectedOSVersion, td.LogBuf.String())
	}
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}

func TestRunTestsGetInitialSysInfo(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	called := false

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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

	td.Cfg.CollectSysInfo = true

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
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
				Name:         "unsupported.Test0",
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
				Name:         "unsupported.Test5",
				Desc:         "This is test 5",
				SoftwareDeps: []string{"has_dep"},
			},
			SkipReason: "dependency not available",
		},
		{
			EntityInfo: internal_testing.EntityInfo{Name: "pkg.Test6", Desc: "This is test 6"},
		},
	}

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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
			count := int64(2)
			for _, t := range testNames {
				mw.WriteMessage(&control.EntityStart{Time: time.Unix(count, 0), Info: testing.EntityInfo{Name: t}})
				count++
				var skipReasons []string
				if strings.HasPrefix(t, "unsupported") {
					skipReasons = append(skipReasons, "dependency not available")
				}
				mw.WriteMessage(&control.EntityEnd{Time: time.Unix(count, 0), Name: t, SkipReasons: skipReasons})
				count++
			}
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(count, 0)})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	// List matching tests instead of running them.
	td.Cfg.LocalDataDir = "/tmp/data"
	td.Cfg.Patterns = []string{"*Test*"}
	td.Cfg.RunLocal = true
	td.Cfg.TotalShards = 2
	td.Cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	expectedPassed := 5
	expectedSkipped := len(tests) - 5
	passed := 0
	skipped := 0
	for shardIndex := 0; shardIndex < td.Cfg.TotalShards; shardIndex++ {
		td.State.SoftwareFeatures = nil
		td.Cfg.ShardIndex = shardIndex
		testResults, err := runTests(context.Background(), &td.Cfg, &td.State, cc)
		if err != nil {
			t.Fatal("Failed to run tests: ", err)
		}
		for _, t := range testResults {
			if t.SkipReason == "" {
				passed++
			} else {
				skipped++
			}
		}
	}
	if passed != expectedPassed {
		t.Errorf("runTests returned %d passed tests; want %d", passed, expectedPassed)
	}
	if skipped != expectedSkipped {
		t.Errorf("runTests returned %d skipped tests; want %d", skipped, expectedSkipped)
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
	td := fakerunner.NewRemoteTestData(t, string(b), "", 0)
	defer td.Close()

	// List matching tests instead of running them.
	td.Cfg.RemoteDataDir = "/tmp/data"
	td.Cfg.Patterns = []string{"*Test*"}
	td.Cfg.RunRemote = true
	td.Cfg.TotalShards = 3

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	processed := make(map[string]bool)
	var state config.State
	for shardIndex := 0; shardIndex < td.Cfg.TotalShards; shardIndex++ {
		td.Cfg.ShardIndex = shardIndex
		testsToRun, testsToSkip, testsNotInShard, err := findTestsForShard(context.Background(), &td.Cfg, &state, cc)
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
