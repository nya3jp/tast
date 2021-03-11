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

	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/dep"
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

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	conn, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	results, err := listLocalTests(context.Background(), &td.cfg, &td.state, conn.SSHConn())
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
	td.cfg.RemoteDataDir = "/tmp/data"
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
	td.cfg.TotalShards = 1
	td.cfg.RunLocal = true

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	results, err := listTests(context.Background(), &td.cfg, &td.state, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := make([]*jsonprotocol.EntityResult, len(tests))
	for i := 0; i < len(tests); i++ {
		expected[i] = &jsonprotocol.EntityResult{EntityInfo: tests[i].EntityInfo, SkipReason: tests[i].SkipReason}
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
	td.cfg.TotalShards = 2
	td.cfg.RunLocal = true

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	for i := 0; i < td.cfg.TotalShards; i++ {
		td.cfg.ShardIndex = i
		results, err := listTests(context.Background(), &td.cfg, &td.state, cc)
		if err != nil {
			t.Error("Failed to list local tests: ", err)
		}
		expected := []*jsonprotocol.EntityResult{
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
	td.cfg.TotalShards = 2
	td.cfg.RunLocal = true

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	// Shard 0 should include all skipped tests.
	td.cfg.ShardIndex = 0
	results, err := listTests(context.Background(), &td.cfg, &td.state, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected := []*jsonprotocol.EntityResult{
		{EntityInfo: tests[0].EntityInfo, SkipReason: tests[0].SkipReason},
		{EntityInfo: tests[2].EntityInfo, SkipReason: tests[2].SkipReason},
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Unexpected list of local tests in shard 0: got %+v; want %+v", results, expected)
	}

	td.cfg.ShardIndex = 1
	// Shard 1 should have only one test
	results, err = listTests(context.Background(), &td.cfg, &td.state, cc)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}
	expected = []*jsonprotocol.EntityResult{
		{EntityInfo: tests[1].EntityInfo, SkipReason: tests[1].SkipReason},
	}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Unexpected list of local tests in shard 1: got %+v; want %+v", results, expected)
	}
}

// TestListTestsGetDUTInfo make sure getDUTInfo is called when listTests is called.
func TestListTestsGetDUTInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

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
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.CheckTestDeps = true

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := listTests(context.Background(), &td.cfg, &td.state, cc); err != nil {
		t.Error("listTests failed: ", err)
	}
	if !called {
		t.Error("runTests did not call getSoftwareFeatures")
	}
}
