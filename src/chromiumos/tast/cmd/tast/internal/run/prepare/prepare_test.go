// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prepare

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"path/filepath"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

func TestPushDataFiles(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	const (
		dataSubdir  = "data" // subdir storing test data per the tast/testing package
		bundleName  = "bnd"  // test bundle
		bundlePkg   = "chromiumos/tast/local/bundles/" + bundleName
		category    = "cat" // test category
		categoryPkg = bundlePkg + "/" + category
		pattern     = "cat.*" // glob matching all tests

		file1        = "file1.txt"
		file2        = "file2.txt"
		file3        = "file3.txt"
		file4        = "file4.txt"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLinkFile1 = extFile1 + testing.ExternalLinkSuffix
		extLinkFile2 = extFile2 + testing.ExternalLinkSuffix

		fixtPkg = "chromiumos/tast/local/chrome"
	)

	// Make local_test_runner list two tests containing the first three files (with overlap).
	tests := []*jsonprotocol.EntityWithRunnabilityInfo{
		{EntityInfo: jsonprotocol.EntityInfo{Name: category + ".Test1", Pkg: categoryPkg, Data: []string{file1, file2}, Fixture: "fixt1"}},
		{EntityInfo: jsonprotocol.EntityInfo{Name: category + ".Test2", Pkg: categoryPkg, Data: []string{file2, file3, extFile1, extFile2}}},
	}
	bundlePath := filepath.Join(fakerunner.MockLocalBundleDir, bundleName)
	fixtures := map[string][]*jsonprotocol.EntityInfo{
		bundlePath: {
			{Type: jsonprotocol.EntityFixture, Name: "fixt1", Pkg: fixtPkg, Data: []string{file1}, Fixture: "fixt2"},
			{Type: jsonprotocol.EntityFixture, Name: "fixt2", Pkg: fixtPkg, Data: []string{file2}},
			{Type: jsonprotocol.EntityFixture, Name: "fixt3", Pkg: fixtPkg, Data: []string{file3}}, // unused
		},
	}

	td.RunFunc = func(args *jsonprotocol.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case jsonprotocol.RunnerListTestsMode:
			fakerunner.CheckArgs(t, args, &jsonprotocol.RunnerArgs{
				Mode: jsonprotocol.RunnerListTestsMode,
				ListTests: &jsonprotocol.RunnerListTestsArgs{
					BundleArgs: jsonprotocol.BundleListTestsArgs{Patterns: []string{pattern}},
					BundleGlob: fakerunner.MockLocalBundleGlob,
				},
			})
			runner.WriteListTestsResultAsJSON(stdout, tests)
		case jsonprotocol.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&jsonprotocol.RunnerListFixturesResult{
				Fixtures: fixtures,
			})
		}
		return 0
	}

	// Create a fake source checkout and write the data files to it. Just use their names as their contents.
	td.Cfg.BuildWorkspace = filepath.Join(td.TempDir, "ws")
	srcFiles := map[string]string{
		file1:        file1,
		file2:        file2,
		file3:        file3,
		file4:        file4,
		extLinkFile1: extLinkFile1,
		extFile2:     extFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(td.Cfg.BuildWorkspace, "src", testing.RelativeDataDir(tests[0].Pkg)), srcFiles); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(filepath.Join(td.Cfg.BuildWorkspace, "src", testing.RelativeDataDir(fixtPkg)), map[string]string{
		file1: file1,
		file2: file2,
		file3: file3,
	}); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	pushDir := filepath.Join(td.HostDir, fakerunner.MockLocalDataDir)
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(pushDir, testing.RelativeDataDir(tests[0].Pkg)), dstFiles); err != nil {
		t.Fatal(err)
	}

	// Connect to the target.
	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	conn, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	td.Cfg.BuildBundle = bundleName
	td.Cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.Cfg, &td.State, conn.SSHConn())
	if err != nil {
		t.Fatal("getDataFilePaths() failed: ", err)
	}
	expPaths := []string{
		filepath.Join(bundlePkg, category, dataSubdir, file1),
		filepath.Join(bundlePkg, category, dataSubdir, file2),
		filepath.Join(bundlePkg, category, dataSubdir, file3),
		filepath.Join(bundlePkg, category, dataSubdir, extFile1),
		filepath.Join(bundlePkg, category, dataSubdir, extFile2),
		filepath.Join(fixtPkg, dataSubdir, file1),
		filepath.Join(fixtPkg, dataSubdir, file2),
	}
	if diff := cmp.Diff(paths, expPaths); diff != "" {
		t.Fatalf("getDataFilePaths() unmatch (-got +want):\n%v", diff)
	}

	// pushDataFiles should copy the required files to the DUT.
	if err = pushDataFiles(context.Background(), &td.Cfg, conn.SSHConn(),
		fakerunner.MockLocalDataDir, paths); err != nil {
		t.Fatal("pushDataFiles() failed: ", err)
	}
	expData := map[string]string{
		filepath.Join(testing.RelativeDataDir(tests[0].Pkg), file1):        file1,
		filepath.Join(testing.RelativeDataDir(tests[0].Pkg), file2):        file2,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), file3):        file3,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), extLinkFile1): extLinkFile1,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), extFile2):     extFile2,
		filepath.Join(testing.RelativeDataDir(fixtPkg), file1):             file1,
		filepath.Join(testing.RelativeDataDir(fixtPkg), file2):             file2,
	}
	if data, err := testutil.ReadFiles(pushDir); err != nil {
		t.Error(err)
	} else if diff := cmp.Diff(data, expData); diff != "" {
		t.Fatalf("pushDataFiles() copied files unmatch (-got +want):\n%v", diff)
	}
	if _, err := ioutil.ReadFile(filepath.Join(pushDir, testing.RelativeDataDir(tests[1].Pkg), extFile1)); err == nil {
		t.Errorf("pushDataFiles() unexpectedly copied %s", extFile1)
	}
}
