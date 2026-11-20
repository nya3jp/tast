// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prepare

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/cmd/tast/internal/run/config"
	"go.chromium.org/tast/core/cmd/tast/internal/run/driver"
	"go.chromium.org/tast/core/cmd/tast/internal/run/runtest"
	"go.chromium.org/tast/core/internal/dep"
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
	paths, err := getDataFilePaths(ctx, cfg, drv, nil)
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

func TestGetDataFilePathsWithFeatures(t *gotesting.T) {
	const (
		categoryPkg = "go.chromium.org/tast-tests/cros/local/bundles/bundle/cat"
		fixtPkg     = "go.chromium.org/tast-tests/cros/local/chrome"
	)
	dutInfo := &protocol.DUTInfo{Features: &fwprotocol.DUTFeatures{
		Software: &fwprotocol.SoftwareFeatures{Available: []string{"mock"}, Unavailable: []string{"dep"}},
	}}

	for _, tc := range []struct {
		name      string
		companion bool
		wantPaths []string
		// wantRelist is whether listing tests to run asks the DUT again.
		wantRelist bool
	}{
		{
			name: "SingleDUT",
			wantPaths: []string{
				filepath.Join(categoryPkg, "data/run.txt"),
				filepath.Join(fixtPkg, "data/fixt_run.txt"),
			},
		},
		{
			// Companion DUT features are not known yet, so tests are listed
			// without features.
			name:      "MultipleDUTs",
			companion: true,
			wantPaths: []string{
				filepath.Join(categoryPkg, "data/run.txt"),
				filepath.Join(categoryPkg, "data/skip.txt"),
				filepath.Join(fixtPkg, "data/fixt_run.txt"),
				filepath.Join(fixtPkg, "data/fixt_skip.txt"),
			},
			wantRelist: true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			reg := testing.NewRegistry("bundle")
			reg.AddTestInstance(&testing.TestInstance{
				Name:    "cat.Run",
				Pkg:     categoryPkg,
				Data:    []string{"run.txt"},
				Fixture: "fixtRun",
			})
			reg.AddTestInstance(&testing.TestInstance{
				Name:         "cat.Skip",
				Pkg:          categoryPkg,
				Data:         []string{"skip.txt"},
				Fixture:      "fixtSkip",
				SoftwareDeps: map[string]dep.SoftwareDeps{"": {"dep"}},
			})
			reg.AddFixtureInstance(&testing.FixtureInstance{Name: "fixtRun", Pkg: fixtPkg, Data: []string{"fixt_run.txt"}})
			reg.AddFixtureInstance(&testing.FixtureInstance{Name: "fixtSkip", Pkg: fixtPkg, Data: []string{"fixt_skip.txt"}})

			opts := []runtest.EnvOrDUTOption{runtest.WithLocalBundles(reg)}
			companions := make(map[string]*fwprotocol.DUTFeatures)
			if tc.companion {
				opts = append(opts, runtest.WithCompanionDUT("cd1"))
				companions["cd1"] = dutInfo.GetFeatures()
			}
			env := runtest.SetUp(t, opts...)
			ctx := env.Context()
			cfg := env.Config(nil)
			drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
			if err != nil {
				t.Fatalf("driver.New failed: %v", err)
			}
			defer drv.Close(ctx)

			paths, err := getDataFilePaths(ctx, cfg, drv, dutInfo)
			if err != nil {
				t.Fatal("getDataFilePaths() failed: ", err)
			}
			slices.Sort(paths)
			if diff := cmp.Diff(paths, tc.wantPaths); diff != "" {
				t.Errorf("getDataFilePaths() mismatch (-got +want):\n%s", diff)
			}

			// A test added to the bundle later is found only if listing tests
			// to run asks the DUT again.
			reg.AddTestInstance(&testing.TestInstance{Name: "cat.Added"})
			tests, err := drv.ListMatchedLocalTests(ctx, cfg.Features(dutInfo.GetFeatures(), companions))
			if err != nil {
				t.Fatal("ListMatchedLocalTests() failed: ", err)
			}
			gotSkips := make(map[string][]string)
			for _, e := range tests {
				gotSkips[e.Resolved.GetEntity().GetName()] = e.Resolved.GetSkip().GetReasons()
			}
			wantSkips := map[string][]string{"cat.Run": nil, "cat.Skip": {"missing SoftwareDeps: dep"}}
			if tc.wantRelist {
				wantSkips["cat.Added"] = nil
			}
			if diff := cmp.Diff(gotSkips, wantSkips); diff != "" {
				t.Errorf("ListMatchedLocalTests() skip reasons mismatch (-got +want):\n%s", diff)
			}
		})
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
			gotDUTInfo, _, err := Prepare(ctx, tc.cfg, tc.drv, nil)
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
			gotPrefetched, err := SetUpRemotePrivateBundle(ctx, tc.cfg, tc.drv)
			if gotPrefetched != nil {
				t.Errorf("SetUpRemotePrivateBundle() returned %+v, want nil", gotPrefetched)
			}

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

func TestSetUpRemotePrivateBundleAndPrepareReuseDUTInfo(t *gotesting.T) {
	const artifactsURL = "gs://example-bucket/build/"
	wantDUTInfo := &protocol.DUTInfo{
		DefaultBuildArtifactsUrl: artifactsURL,
		OsVersion:                "R160-16891.0.0",
	}

	var getDUTInfoCalls, localCalls int
	var remoteURL, localURL string
	env := runtest.SetUp(
		t,
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			getDUTInfoCalls++
			return &protocol.GetDUTInfoResponse{DutInfo: wantDUTInfo}, nil
		}),
		runtest.WithDownloadPrivateRemoteBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			remoteURL = req.GetBuildArtifactUrl()
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			localCalls++
			localURL = req.GetBuildArtifactUrl()
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Build = false
		cfg.DownloadPrivateBundles = true
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	prefetched, err := SetUpRemotePrivateBundle(ctx, cfg, drv)
	if err != nil {
		t.Fatalf("SetUpRemotePrivateBundle() failed: %v", err)
	}
	if prefetched == nil {
		t.Fatal("SetUpRemotePrivateBundle() returned nil, want non-nil *Prefetched")
	}
	if diff := cmp.Diff(prefetched.dutInfo, wantDUTInfo, protocmp.Transform()); diff != "" {
		t.Fatalf("SetUpRemotePrivateBundle() DUTInfo mismatch (-got +want):\n%s", diff)
	}
	if remoteURL != artifactsURL {
		t.Errorf("DownloadPrivateRemoteBundles URL = %q, want %q", remoteURL, artifactsURL)
	}
	if localURL != artifactsURL {
		t.Errorf("DownloadPrivateLocalBundles URL = %q, want %q", localURL, artifactsURL)
	}

	gotDUTInfo, _, err := Prepare(ctx, cfg, drv, prefetched)
	if err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}
	if diff := cmp.Diff(gotDUTInfo, wantDUTInfo, protocmp.Transform()); diff != "" {
		t.Errorf("Prepare() DUTInfo mismatch (-got +want):\n%s", diff)
	}
	if getDUTInfoCalls != 1 {
		t.Errorf("GetDUTInfo calls = %d, want 1", getDUTInfoCalls)
	}
	if localCalls != 1 {
		t.Errorf("DownloadPrivateLocalBundles calls = %d, want 1", localCalls)
	}

	// Passing nil should query GetDUTInfo and download private local bundles
	// again.
	localURL = ""
	gotDUTInfo, _, err = Prepare(ctx, cfg, drv, nil)
	if err != nil {
		t.Fatalf("Prepare(nil) failed: %v", err)
	}
	if diff := cmp.Diff(gotDUTInfo, wantDUTInfo, protocmp.Transform()); diff != "" {
		t.Errorf("Prepare(nil) DUTInfo mismatch (-got +want):\n%s", diff)
	}
	if localURL != artifactsURL {
		t.Errorf("DownloadPrivateLocalBundles URL after Prepare(nil) = %q, want %q", localURL, artifactsURL)
	}
	if getDUTInfoCalls != 2 {
		t.Errorf("GetDUTInfo calls after Prepare(nil) = %d, want 2", getDUTInfoCalls)
	}
	if localCalls != 2 {
		t.Errorf("DownloadPrivateLocalBundles calls after Prepare(nil) = %d, want 2", localCalls)
	}
}

func TestSetUpRemotePrivateBundleDownloadsConcurrently(t *gotesting.T) {
	// Each download handler waits for the other one to start, which happens
	// only if the local and remote downloads run concurrently.
	localStarted := make(chan struct{})
	remoteStarted := make(chan struct{})
	waitFor := func(ch <-chan struct{}, name string) {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Errorf("%s download did not start concurrently", name)
		}
	}
	env := runtest.SetUp(
		t,
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{DefaultBuildArtifactsUrl: "gs://example-bucket/build/"}}, nil
		}),
		runtest.WithDownloadPrivateRemoteBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			close(remoteStarted)
			waitFor(localStarted, "Local")
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			close(localStarted)
			waitFor(remoteStarted, "Remote")
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Build = false
		cfg.DownloadPrivateBundles = true
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if _, err := SetUpRemotePrivateBundle(ctx, cfg, drv); err != nil {
		t.Fatalf("SetUpRemotePrivateBundle() failed: %v", err)
	}
}

func TestSetUpRemotePrivateBundleWithoutDUT(t *gotesting.T) {
	const artifactsURL = "gs://example-bucket/override/"
	var remoteURL string
	var localCalls int
	env := runtest.SetUp(
		t,
		runtest.WithDownloadPrivateRemoteBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			remoteURL = req.GetBuildArtifactUrl()
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			localCalls++
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Target = "-"
		cfg.Build = false
		cfg.DownloadPrivateBundles = true
		cfg.BuildArtifactsURLOverride = artifactsURL
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	prefetched, err := SetUpRemotePrivateBundle(ctx, cfg, drv)
	if err != nil {
		t.Fatalf("SetUpRemotePrivateBundle() failed: %v", err)
	}
	if prefetched != nil {
		t.Errorf("SetUpRemotePrivateBundle() returned %+v, want nil", prefetched)
	}
	if remoteURL != artifactsURL {
		t.Errorf("DownloadPrivateRemoteBundles URL = %q, want %q", remoteURL, artifactsURL)
	}
	if localCalls != 0 {
		t.Errorf("DownloadPrivateLocalBundles calls = %d, want 0", localCalls)
	}
}

func TestSetUpRemotePrivateBundleLocalDownloadError(t *gotesting.T) {
	localErr := errors.New("local download failure")
	var localCalls int
	env := runtest.SetUp(
		t,
		runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{DefaultBuildArtifactsUrl: "gs://example-bucket/build/"}}, nil
		}),
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			localCalls++
			return nil, localErr
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Build = false
		cfg.DownloadPrivateBundles = true
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	// SetUpRemotePrivateBundle leaves reporting the local download error to
	// Prepare.
	prefetched, err := SetUpRemotePrivateBundle(ctx, cfg, drv)
	if err != nil {
		t.Fatalf("SetUpRemotePrivateBundle() failed: %v", err)
	}
	const wantErr = "failed downloading private bundles: "
	_, _, err = Prepare(ctx, cfg, drv, prefetched)
	if err == nil || !strings.Contains(err.Error(), wantErr) || !strings.Contains(err.Error(), localErr.Error()) {
		t.Errorf("Prepare() returned error %v, want one containing %q and %q", err, wantErr, localErr)
	}
	if localCalls != 1 {
		t.Errorf("DownloadPrivateLocalBundles calls = %d, want 1", localCalls)
	}
}

func TestSetUpRemotePrivateBundleRemoteDownloadError(t *gotesting.T) {
	remoteErr := errors.New("remote download failure")
	for _, tc := range []struct {
		name string
		// localErr is returned by the local download. If nil, the local
		// download is still in progress when the remote download fails.
		localErr error
	}{
		{name: "local_in_progress"},
		{name: "both_fail", localErr: errors.New("local download failure")},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			localStarted := make(chan struct{})
			localReturned := make(chan struct{})
			release := make(chan struct{})
			var localTimedOut bool
			env := runtest.SetUp(
				t,
				runtest.WithGetDUTInfo(func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
					return &protocol.GetDUTInfoResponse{DutInfo: &protocol.DUTInfo{DefaultBuildArtifactsUrl: "gs://example-bucket/build/"}}, nil
				}),
				runtest.WithDownloadPrivateRemoteBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
					// Fail while the local download is in progress, or after
					// it failed.
					wait, waitDesc := localStarted, "start"
					if tc.localErr != nil {
						wait, waitDesc = localReturned, "return"
					}
					select {
					case <-wait:
					case <-time.After(10 * time.Second):
						t.Errorf("Local download did not %s", waitDesc)
					}
					return nil, remoteErr
				}),
				runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
					defer close(localReturned)
					close(localStarted)
					if tc.localErr != nil {
						return nil, tc.localErr
					}
					select {
					case <-release:
					case <-time.After(10 * time.Second):
						localTimedOut = true
					}
					return &protocol.DownloadPrivateBundlesResponse{}, nil
				}),
			)
			ctx := env.Context()
			cfg := env.Config(func(cfg *config.MutableConfig) {
				cfg.Build = false
				cfg.DownloadPrivateBundles = true
			})

			drv, err := driver.New(ctx, cfg, cfg.Target(), "", nil)
			if err != nil {
				t.Fatalf("driver.New failed: %v", err)
			}
			defer drv.Close(ctx)

			prefetched, err := SetUpRemotePrivateBundle(ctx, cfg, drv)
			close(release)
			select {
			case <-localReturned:
			case <-time.After(10 * time.Second):
				t.Fatal("Local download did not return")
			}
			// The local download in progress is canceled instead of awaited.
			if localTimedOut {
				t.Error("SetUpRemotePrivateBundle() waited for the local download to complete")
			}
			const wantErr = "failed downloading Remote private bundles: "
			if err == nil || !strings.Contains(err.Error(), wantErr) || !strings.Contains(err.Error(), remoteErr.Error()) {
				t.Errorf("SetUpRemotePrivateBundle() returned error %v, want one containing %q and %q", err, wantErr, remoteErr)
			}
			if prefetched != nil {
				t.Errorf("SetUpRemotePrivateBundle() returned %+v, want nil", prefetched)
			}
		})
	}
}
