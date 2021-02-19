// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	gotesting "testing"

	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
)

func TestListLocalTests(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:      runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{BundleGlob: mockLocalBundleGlob},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}

	hst, err := connectToTarget(context.Background(), &td.cfg, &td.state)
	if err != nil {
		t.Fatal(err)
	}

	results, err := listLocalTests(context.Background(), &td.cfg, &td.state, hst)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}

	if !reflect.DeepEqual(results, tests) {
		t.Errorf("Unexpected list of local tests: got %+v; want %+v", results, tests)
	}
}

func TestListRemoteList(t *gotesting.T) {
	// Make the runner print serialized tests.
	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test1",
				Desc: "First description",
				Attr: []string{"attr1", "attr2"},
				Pkg:  "pkg",
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg2.Test2",
				Desc: "Second description",
				Attr: []string{"attr3"},
				Pkg:  "pkg2",
			},
		},
	}
	b, err := json.Marshal(&tests)
	if err != nil {
		t.Fatal(err)
	}
	td := newRemoteTestData(t, string(b), "", 0)
	defer td.close()

	// List matching tests instead of running them.
	td.cfg.remoteDataDir = "/tmp/data"
	td.cfg.Patterns = []string{"*Test*"}

	results, err := listRemoteTests(context.Background(), &td.cfg, &td.state)
	if err != nil {
		t.Error("Failed to list remote tests: ", err)
	}

	if !reflect.DeepEqual(results, tests) {
		t.Errorf("Unexpected list of remote tests: got %+v; want %+v", results, tests)
	}
}

// TestListTests make sure list test can list all tests.
func TestListTests(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:      runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{BundleGlob: mockLocalBundleGlob},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}
	td.cfg.totalShards = 1
	td.cfg.runLocal = true

	results, err := listTests(context.Background(), &td.cfg, &td.state)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := make([]*EntityResult, len(tests))
	for i := 0; i < len(tests); i++ {
		expected[i] = &EntityResult{EntityInfo: tests[i].EntityInfo, SkipReason: tests[i].SkipReason}
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Unexpected list of local tests: got %+v; want %+v", results, expected)
	}
}

// TestListTestsWithSharding make sure list test can list tests in specified shards.
func TestListTestsWithSharding(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:      runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{BundleGlob: mockLocalBundleGlob},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}
	td.cfg.totalShards = 2
	td.cfg.runLocal = true

	for i := 0; i < td.cfg.totalShards; i++ {
		td.cfg.shardIndex = i
		results, err := listTests(context.Background(), &td.cfg, &td.state)
		if err != nil {
			t.Error("Failed to list local tests: ", err)
		}
		expected := []*EntityResult{
			{EntityInfo: tests[i].EntityInfo, SkipReason: tests[i].SkipReason},
		}
		if !reflect.DeepEqual(results, expected) {
			t.Errorf("Unexpected list of local tests: got %+v; want %+v", results, expected)
		}
	}
}

// TestListTestsWithSkippedTests make sure list test can list skipped tests correctly.
func TestListTestsWithSkippedTests(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.EntityWithRunnabilityInfo{
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.Test",
				Desc: "This is a test",
				Attr: []string{"attr1", "attr2"},
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.AnotherTest",
				Desc: "Another test",
			},
		},
		{
			EntityInfo: testing.EntityInfo{
				Name: "pkg.SkippedTest",
				Desc: "Skipped test",
			},
			SkipReason: "Skip",
		},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:      runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{BundleGlob: mockLocalBundleGlob},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}
	td.cfg.totalShards = 2
	td.cfg.runLocal = true

	// Shard 0 should include all skipped tests.
	td.cfg.shardIndex = 0
	results, err := listTests(context.Background(), &td.cfg, &td.state)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := []*EntityResult{
		{EntityInfo: tests[0].EntityInfo, SkipReason: tests[0].SkipReason},
		{EntityInfo: tests[2].EntityInfo, SkipReason: tests[2].SkipReason},
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Unexpected list of local tests in shard 0: got %+v; want %+v", results, expected)
	}

	td.cfg.shardIndex = 1
	// Shard 1 should have only one test
	results, err = listTests(context.Background(), &td.cfg, &td.state)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected = []*EntityResult{
		{EntityInfo: tests[1].EntityInfo, SkipReason: tests[1].SkipReason},
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Unexpected list of local tests in shard 1: got %+v; want %+v", results, expected)
	}
}
