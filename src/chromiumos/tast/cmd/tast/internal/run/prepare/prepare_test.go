// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prepare

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

func TestPushDataFiles(t *gotesting.T) {
	const (
		dataSubdir  = "data"   // subdir storing test data per the tast/testing package
		bundleName  = "bundle" // test bundle
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

	// Make the local test bundle list two tests containing the first three files (with overlap).
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name:    category + ".Test1",
		Pkg:     categoryPkg,
		Data:    []string{file1, file2},
		Fixture: "fixt1",
	})
	reg.AddTestInstance(&testing.TestInstance{
		Name: category + ".Test2",
		Pkg:  categoryPkg,
		Data: []string{file2, file3, extFile1, extFile2},
	})
	reg.AddFixtureInstance(&testing.FixtureInstance{
		Name:   "fixt1",
		Pkg:    fixtPkg,
		Data:   []string{file1},
		Parent: "fixt2",
	})
	reg.AddFixtureInstance(&testing.FixtureInstance{
		Name: "fixt2",
		Pkg:  fixtPkg,
		Data: []string{file2},
	})
	reg.AddFixtureInstance(&testing.FixtureInstance{
		Name: "fixt3",
		Pkg:  fixtPkg,
		Data: []string{file3},
	})

	env := runtest.SetUp(t, runtest.WithLocalBundles(reg), runtest.WithExtraSSHHandlers([]runtest.SSHHandler{
		// Allow rm commands with relative paths.
		func(cmd string) (runtest.SSHProcess, bool) {
			if !regexp.MustCompile(`^cd .*; exec rm `).MatchString(cmd) {
				return nil, false
			}
			return runtest.ShellHandler("")(cmd)
		},
	}))
	cfg := env.Config()
	state := env.State()

	// Create a fake source checkout and write the data files to it. Just use their names as their contents.
	cfg.BuildWorkspace = filepath.Join(env.TempDir(), "ws")
	srcFiles := map[string]string{
		file1:        file1,
		file2:        file2,
		file3:        file3,
		file4:        file4,
		extLinkFile1: extLinkFile1,
		extFile2:     extFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(cfg.BuildWorkspace, "src", testing.RelativeDataDir(categoryPkg)), srcFiles); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(filepath.Join(cfg.BuildWorkspace, "src", testing.RelativeDataDir(fixtPkg)), map[string]string{
		file1: file1,
		file2: file2,
		file3: file3,
	}); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	localDataDir := filepath.Join(env.TempDir(), "mock_local_data")
	cfg.LocalDataDir = localDataDir
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(localDataDir, testing.RelativeDataDir(categoryPkg)), dstFiles); err != nil {
		t.Fatal(err)
	}

	// Connect to the target.
	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(context.Background())

	conn, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	cfg.BuildBundle = bundleName
	cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), cfg, state, conn.SSHConn())
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
	if err = pushDataFiles(context.Background(), cfg, conn.SSHConn(), localDataDir, paths); err != nil {
		t.Fatal("pushDataFiles() failed: ", err)
	}
	expData := map[string]string{
		filepath.Join(testing.RelativeDataDir(categoryPkg), file1):        file1,
		filepath.Join(testing.RelativeDataDir(categoryPkg), file2):        file2,
		filepath.Join(testing.RelativeDataDir(categoryPkg), file3):        file3,
		filepath.Join(testing.RelativeDataDir(categoryPkg), extLinkFile1): extLinkFile1,
		filepath.Join(testing.RelativeDataDir(categoryPkg), extFile2):     extFile2,
		filepath.Join(testing.RelativeDataDir(fixtPkg), file1):            file1,
		filepath.Join(testing.RelativeDataDir(fixtPkg), file2):            file2,
	}
	if data, err := testutil.ReadFiles(localDataDir); err != nil {
		t.Error(err)
	} else if diff := cmp.Diff(data, expData); diff != "" {
		t.Fatalf("pushDataFiles() copied files unmatch (-got +want):\n%v", diff)
	}
	if _, err := ioutil.ReadFile(filepath.Join(localDataDir, testing.RelativeDataDir(categoryPkg), extFile1)); err == nil {
		t.Errorf("pushDataFiles() unexpectedly copied %s", extFile1)
	}
}
