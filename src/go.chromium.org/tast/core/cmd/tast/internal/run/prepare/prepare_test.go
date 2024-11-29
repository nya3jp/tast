// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prepare

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/runtest"
	"go.chromium.org/tast/core/internal/fakesshserver"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"

	fwprotocol "go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/testutil"
)

func TestPushDataFiles(t *gotesting.T) {
	const (
		dataSubdir  = "data"   // subdir storing test data per the tast/testing package
		bundleName  = "bundle" // test bundle
		bundlePkg   = "go.chromium.org/tast-tests/cros/local/bundles/" + bundleName
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

		fixtPkg = "go.chromium.org/tast-tests/cros/local/chrome"
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

	env := runtest.SetUp(t, runtest.WithLocalBundles(reg), runtest.WithExtraSSHHandlers([]fakesshserver.Handler{
		// Allow rm commands with relative paths.
		func(cmd string) (fakesshserver.Process, bool) {
			if !regexp.MustCompile(`^cd .*; exec rm `).MatchString(cmd) {
				return nil, false
			}
			return fakesshserver.ShellHandler("")(cmd)
		},
	}))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.BuildWorkspace = filepath.Join(env.TempDir(), "ws")
		cfg.LocalDataDir = filepath.Join(env.TempDir(), "mock_local_data")
		cfg.BuildBundle = bundleName
		cfg.Patterns = []string{pattern}
	})

	// Create a fake source checkout and write the data files to it. Just use their names as their contents.
	srcFiles := map[string]string{
		file1:        file1,
		file2:        file2,
		file3:        file3,
		file4:        file4,
		extLinkFile1: extLinkFile1,
		extFile2:     extFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(cfg.BuildWorkspace(), "src", testing.RelativeDataDir(categoryPkg)), srcFiles); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(filepath.Join(cfg.BuildWorkspace(), "src", testing.RelativeDataDir(fixtPkg)), map[string]string{
		file1: file1,
		file2: file2,
		file3: file3,
	}); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(cfg.LocalDataDir(), testing.RelativeDataDir(categoryPkg)), dstFiles); err != nil {
		t.Fatal(err)
	}

	// Connect to the target.
	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	// getDataFilePaths should list the tests and return the files needed by them.
	paths, err := getDataFilePaths(ctx, cfg, drv)
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
	if err = pushDataFiles(ctx, cfg, drv.SSHConn(), cfg.LocalDataDir(), paths); err != nil {
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
	if data, err := testutil.ReadFiles(cfg.LocalDataDir()); err != nil {
		t.Error(err)
	} else if diff := cmp.Diff(data, expData); diff != "" {
		t.Fatalf("pushDataFiles() copied files unmatch (-got +want):\n%v", diff)
	}
	if _, err := os.ReadFile(filepath.Join(cfg.LocalDataDir(), testing.RelativeDataDir(categoryPkg), extFile1)); err == nil {
		t.Errorf("pushDataFiles() unexpectedly copied %s", extFile1)
	}
}

func TestPrepare(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	tests := []struct {
		name string
		cfg  *config.Config
		drv  *driver.Driver
	}{
		{
			name: "PrepareNoHostNoBuild",
			cfg: env.Config(func(cfg *config.MutableConfig) {
				cfg.Target = "-"
				cfg.Build = false
			}),
			drv: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *gotesting.T) {
			gotDUTInfo, _, err := Prepare(ctx, tc.cfg, tc.drv)
			if err != nil {
				t.Errorf("Unexpected error in Prepare(): %v", err)
			}

			wantDUTInfo := &protocol.DUTInfo{
				Features: &fwprotocol.DUTFeatures{
					Software: &fwprotocol.SoftwareFeatures{},
					Hardware: &fwprotocol.HardwareFeatures{},
				},
			}

			if diff := cmp.Diff(wantDUTInfo, gotDUTInfo, protocmp.Transform()); diff != "" {
				t.Errorf("Prepare(): Unwanted diff (-want +want):\n%s", diff)
			}
		})
	}
}

func TestSetupPrivateBundles(t *gotesting.T) {
	env := runtest.SetUp(t)
	ctx := env.Context()
	tests := []struct {
		name    string
		cfg     *config.Config
		drv     *driver.Driver
		wantErr error
	}{
		{
			name: "DriverIsNil",
			cfg: env.Config(func(cfg *config.MutableConfig) {
				cfg.Target = "-"
				cfg.Build = false
			}),
			drv:     nil,
			wantErr: errors.New("driver is nil"),
		},
		{
			name: "NoBuildNoDownloadNoChroot",
			cfg: env.Config(func(cfg *config.MutableConfig) {
				cfg.Target = "-"
				cfg.Build = false
				cfg.DownloadPrivateBundles = false
			}),
			drv:     &driver.Driver{},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *gotesting.T) {
			err := SetUpRemotePrivateBundle(ctx, tc.cfg, tc.drv)

			if tc.wantErr != nil {
				if err == nil {
					t.Errorf("SetupPrivateBundles() expected an error, but got nil")
				} else if err.Error() != tc.wantErr.Error() {
					t.Errorf("SetupPrivateBundles() error mismatch: got %q, want %q", err.Error(), tc.wantErr.Error())
				}
			} else {
				if err != nil {
					t.Errorf("SetupPrivateBundles() unexpected error: %v", err)
				}
			}
		})
	}
}
