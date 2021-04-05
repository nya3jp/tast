// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/testutil"
)

// runFakeRemoteRunner calls remote and records the Args struct that was passed
// to the fake runner.
func runFakeRemoteRunner(t *gotesting.T, td *fakerunner.RemoteTestData) ([]*resultsjson.Result, error) {
	res, rerr := RunRemoteTests(context.Background(), &td.Cfg, &td.State)

	f, err := os.Open(filepath.Join(td.Dir, fakerunner.FakeRunnerArgsFile))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err = json.NewDecoder(f).Decode(&td.Args); err != nil {
		t.Fatal(err)
	}

	return res, rerr
}

func TestRemoteRun(t *gotesting.T) {
	const testName = "pkg.Test"

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: jsonprotocol.EntityInfo{Name: testName}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""})

	td := fakerunner.NewRemoteTestData(t, b.String(), "", 0)
	defer td.Close()

	// Set some parameters that can be overridden by flags to arbitrary values.
	td.Cfg.Build = false
	td.Cfg.KeyFile = "/tmp/id_dsa"
	td.Cfg.RemoteBundleDir = "/tmp/bundles"
	td.Cfg.RemoteDataDir = "/tmp/data"
	td.Cfg.RemoteOutDir = "/tmp/out"
	td.Cfg.BuildArtifactsURL = fakerunner.MockBuildArtifactsURL
	td.Cfg.TLWServer = "tlwserver"

	res, err := runFakeRemoteRunner(t, td)
	if err != nil {
		t.Errorf("RunRemoteTests(%+v) failed: %v", td.Cfg, err)
	}
	if len(res) != 1 {
		t.Errorf("RunRemoteTests(%+v) returned %v result(s); want 1", td.Cfg, len(res))
	} else if res[0].Name != testName {
		t.Errorf("RunRemoteTests(%+v) returned result for test %q; want %q", td.Cfg, res[0].Name, testName)
	}

	glob := filepath.Join(td.Cfg.RemoteBundleDir, "*")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runFlags := []string{
		"-build=false", // propagated from td.cfg.build
		"-keyfile=" + td.Cfg.KeyFile,
		"-keydir=",
		"-remoterunner=" + td.Cfg.RemoteRunner,
		"-remotebundledir=" + td.Cfg.RemoteBundleDir,
		"-remotedatadir=" + td.Cfg.RemoteDataDir,
		"-localrunner=" + td.Cfg.LocalRunner,
		"-localbundledir=" + td.Cfg.LocalBundleDir,
		"-localdatadir=" + td.Cfg.LocalDataDir,
		"-devservers=" + strings.Join(td.Cfg.Devservers, ","),
		"-buildartifactsurl=" + fakerunner.MockBuildArtifactsURL,
	}
	expArgs := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleGlob: glob,
			BundleArgs: bundle.BundleRunTestsArgs{
				DataDir:        td.Cfg.RemoteDataDir,
				OutDir:         td.Cfg.RemoteOutDir,
				KeyFile:        td.Cfg.KeyFile,
				TastPath:       exe,
				RunFlags:       runFlags,
				LocalBundleDir: fakerunner.MockLocalBundleDir,
				FeatureArgs: bundle.FeatureArgs{
					CheckSoftwareDeps: false,
				},
				Devservers:        fakerunner.MockDevservers,
				TLWServer:         td.Cfg.TLWServer,
				BuildArtifactsURL: fakerunner.MockBuildArtifactsURL,
				DownloadMode:      planner.DownloadLazy,
				HeartbeatInterval: heartbeatInterval,
			},
			BuildArtifactsURLDeprecated: fakerunner.MockBuildArtifactsURL,
		},
	}
	if diff := cmp.Diff(td.Args, expArgs, cmpopts.IgnoreUnexported(expArgs)); diff != "" {
		t.Errorf("RunRemoteTests(%+v) passed unexpected args; diff (-got +want):\n%s", td.Cfg, diff)
	}
}

func TestRemoteRunCopyOutput(t *gotesting.T) {
	const (
		testName = "pkg.Test"
		outFile  = "somefile.txt"
		outData  = "somedata"
		outName  = "pkg.Test.tmp1234"
	)

	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: jsonprotocol.EntityInfo{Name: testName}, OutDir: filepath.Join(outDir, outName)})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: outDir})

	td := fakerunner.NewRemoteTestData(t, b.String(), "", 0)
	defer td.Close()

	// Set some parameters that can be overridden by flags to arbitrary values.
	td.Cfg.KeyFile = "/tmp/id_dsa"
	td.Cfg.RemoteBundleDir = "/tmp/bundles"
	td.Cfg.RemoteDataDir = "/tmp/data"
	td.Cfg.RemoteOutDir = outDir

	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := runFakeRemoteRunner(t, td); err != nil {
		t.Errorf("RunRemoteTests(%+v) failed: %v", td.Cfg, err)
	}

	files, err := testutil.ReadFiles(filepath.Join(td.Cfg.ResDir, testLogsDir))
	if err != nil {
		t.Fatal(err)
	}
	if out, ok := files[filepath.Join(testName, outFile)]; !ok {
		t.Errorf("%s was not created", filepath.Join(testName, outFile))
	} else if out != outData {
		t.Errorf("%s was corrupted: got %q, want %q", filepath.Join(testName, outFile), out, outData)
	}
}

// disabledTestRemoteFailure is temporarily disabled.
// TODO(crbug.com/1003952): Re-enable this test after fixing a race condition.
func disabledTestRemoteFailure(t *gotesting.T) {
	// Make the test runner print a message to stderr and fail.
	const errorMsg = "Whoops, something failed\n"
	td := fakerunner.NewRemoteTestData(t, "", errorMsg, 1)
	defer td.Close()

	if _, err := runFakeRemoteRunner(t, td); err == nil {
		t.Errorf("RunRemoteTests(%v) unexpectedly passed", td.Cfg)
	} else if !strings.Contains(err.Error(), strings.TrimRight(errorMsg, "\n")) {
		// The runner's error message should've been logged.
		t.Errorf("RunRemoteTests(%v) didn't log runner error %q in %q", td.Cfg, errorMsg, err.Error())
	}
}

// TestRemoteMaxFailures makes sure that RunRemoteTests does not run any tests if maximum failures allowed has been reach.
func TestRemoteMaxFailures(t *gotesting.T) {
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 2})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: jsonprotocol.EntityInfo{Name: "t1"}})
	mw.WriteMessage(&control.EntityError{Time: time.Unix(3, 0), Name: "t1", Error: jsonprotocol.Error{Reason: "error"}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(4, 0), Name: "t1"})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(5, 0), Info: jsonprotocol.EntityInfo{Name: "t2"}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(6, 0), Name: "t2"})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(7, 0), OutDir: ""})

	td := fakerunner.NewRemoteTestData(t, b.String(), "", 0)
	defer td.Close()

	td.Cfg.MaxTestFailures = 1
	td.State.FailuresCount = 0

	results, err := RunRemoteTests(context.Background(), &td.Cfg, &td.State)
	if err == nil {
		t.Errorf("RunRemoteTests(%+v, %+v) passed unexpectedly", td.Cfg, td.State)
	}
	if len(results) != 1 {
		t.Errorf("RunRemoteTests return %v results; want 1", len(results))
	}
}

// TODO(derat): Add a test that verifies that GetInitialSysInfo is called before tests are run.
// Also verify that state.StartedRun is false if we see an early failure during GetInitialSysInfo.
