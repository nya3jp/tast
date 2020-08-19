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

	tests := []testing.EntityInfo{
		{Name: "pkg.Test", Desc: "This is a test", Attr: []string{"attr1", "attr2"}},
		{Name: "pkg.AnotherTest", Desc: "Another test"},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:      runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{BundleGlob: mockLocalBundleGlob},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}

	hst, err := connectToTarget(context.Background(), &td.cfg)
	if err != nil {
		t.Fatal(err)
	}

	results, err := listLocalTests(context.Background(), &td.cfg, hst)
	if err != nil {
		t.Error("Failed to list local tests: ", err)
	}

	if !reflect.DeepEqual(results, tests) {
		t.Errorf("Unexpected list of local tests: got %+v; want %+v", results, tests)
	}
}

func TestListRemoteList(t *gotesting.T) {
	// Make the runner print serialized tests.
	tests := []testing.EntityInfo{
		{Name: "pkg.Test1", Desc: "First description", Attr: []string{"attr1", "attr2"}, Pkg: "pkg"},
		{Name: "pkg2.Test2", Desc: "Second description", Attr: []string{"attr3"}, Pkg: "pkg2"},
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

	results, err := listRemoteTests(context.Background(), &td.cfg)
	if err != nil {
		t.Error("Failed to list remote tests: ", err)
	}

	if !reflect.DeepEqual(results, tests) {
		t.Errorf("Unexpected list of remote tests: got %+v; want %+v", results, tests)
	}
}
