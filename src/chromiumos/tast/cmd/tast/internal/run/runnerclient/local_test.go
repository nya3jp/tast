// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testutil"
)

func TestLocalSuccess(t *gotesting.T) {
	t.Parallel()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			fakerunner.CheckArgs(t, args, &runner.RunnerArgs{
				RunTests: &runner.RunnerRunTestsArgs{
					BundleArgs: jsonprotocol.BundleRunTestsArgs{
						DataDir:           fakerunner.MockLocalDataDir,
						OutDir:            fakerunner.MockLocalOutDir,
						Devservers:        fakerunner.MockDevservers,
						DUTName:           td.Cfg.Target,
						BuildArtifactsURL: fakerunner.MockBuildArtifactsURL,
						DownloadMode:      planner.DownloadLazy,
						HeartbeatInterval: heartbeatInterval,
					},
					BundleGlob:                  fakerunner.MockLocalBundleGlob,
					Devservers:                  fakerunner.MockDevservers,
					BuildArtifactsURLDeprecated: fakerunner.MockBuildArtifactsURL,
				},
			})

			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.RunnerListFixturesResult{})
		}
		return 0
	}

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // avoid test being blocked indefinitely
	defer cancel()

	if _, err := RunLocalTests(ctx, &td.Cfg, &td.State, cc); err != nil {
		t.Errorf("runLocalTest failed: %v", err)
	}
}

func TestLocalProxy(t *gotesting.T) {
	t.Parallel()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.RunnerListFixturesResult{})
		}
		return 0
	}

	// Configure proxy settings to forward to the DUT.
	const (
		httpProxy  = "10.0.0.1:8000"
		httpsProxy = "10.0.0.1:8001"
		noProxy    = "foo.com, localhost, 127.0.0.0"
	)
	for name, val := range map[string]string{
		"HTTP_PROXY":  httpProxy,
		"HTTPS_PROXY": httpsProxy,
		"NO_PROXY":    noProxy,
	} {
		old := os.Getenv(name)
		if err := os.Setenv(name, val); err != nil {
			t.Fatal(err)
		}
		if old != "" {
			defer os.Setenv(name, old)
		}
	}
	td.Cfg.Proxy = config.ProxyEnv

	// Proxy environment variables should be prepended to the local_test_runner command line.
	// (The variables are added in this order in local.go.)
	td.ExpRunCmd = strings.Join([]string{
		"exec",
		"env",
		shutil.Escape("HTTP_PROXY=" + httpProxy),
		shutil.Escape("HTTPS_PROXY=" + httpsProxy),
		shutil.Escape("NO_PROXY=" + noProxy),
		fakerunner.MockLocalRunner,
	}, " ")

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Error("RunLocalTests failed: ", err)
	}
}

func TestLocalCopyOutput(t *gotesting.T) {
	const (
		testName = "pkg.Test"
		outFile  = "somefile.txt"
		outData  = "somedata"
		outName  = "pkg.Test.tmp1234"
	)

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{testName}})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: jsonprotocol.EntityInfo{Name: testName}, OutDir: filepath.Join(td.Cfg.LocalOutDir, outName)})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: td.Cfg.LocalOutDir})
		case runner.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.RunnerListFixturesResult{})
		}
		return 0
	}

	if err := testutil.WriteFiles(filepath.Join(td.HostDir, td.Cfg.LocalOutDir), map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	td.Cfg.TestsToRun = []*resultsjson.Result{{Test: resultsjson.Test{
		Name: testName,
	}}}

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("RunLocalTests failed: %v", err)
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

func disabledTestLocalExecFailure(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	const msg = "some failure message\n"

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		io.WriteString(stderr, msg)
		return 1
	}

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Error("RunLocalTests unexpectedly passed")
	}
	if !strings.Contains(td.LogBuf.String(), msg) {
		t.Errorf("RunLocalTests logged %q; want substring %q", td.LogBuf.String(), msg)
	}
}

func TestLocalWaitTimeout(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// Simulate local_test_runner writing control messages immediately but hanging before exiting.
	td.RunDelay = time.Minute
	td.Cfg.TestsToRun = []*resultsjson.Result{{Test: resultsjson.Test{Name: "pkg.Foo"}}}
	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0)})
		case runner.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.RunnerListFixturesResult{})
		}
		return 0
	}

	// After setting a short wait timeout, an error should be reported.
	td.Cfg.LocalRunnerWaitTimeout = time.Millisecond

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	if _, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Error("RunLocalTests unexpectedly passed")
	}
}

// TestLocalMaxFailures makes sure that RunLocalTests does not run any tests after maximum failures allowed has been reach.
func TestLocalMaxFailures(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 2})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: jsonprotocol.EntityInfo{Name: "t1"}})
			mw.WriteMessage(&control.EntityError{Time: time.Unix(3, 0), Name: "t1", Error: jsonprotocol.Error{Reason: "error"}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(4, 0), Name: "t1"})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(5, 0), Info: jsonprotocol.EntityInfo{Name: "t2"}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(6, 0), Name: "t2"})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(7, 0), OutDir: ""})
		case runner.RunnerListFixturesMode:
			fmt.Fprintln(stdout, "{}") // no fixtures
		}
		return 0
	}
	td.Cfg.TestsToRun = []*resultsjson.Result{{Test: resultsjson.Test{Name: "pkg.Test"}}}
	td.Cfg.MaxTestFailures = 1
	td.State.FailuresCount = 0

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	results, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc)
	if err == nil {
		t.Errorf("RunLocalTests() passed unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("RunLocalTests return %v results; want 1", len(results))
	}
}

func TestFixturesDependency(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t, fakerunner.WithFakeRemoteRunnerData([]*jsonprotocol.EntityInfo{
		{Name: "remoteFixt"},
		{Name: "failFixt"},
		{Name: "tearDownFailFixt"},
	}), fakerunner.WithFakeRemoteServerData(&fakerunner.FakeRemoteServerData{
		Fixtures: map[string]*fakerunner.SerializableFakeFixture{
			"remoteFixt":       {SetUpLog: "Hello", TearDownLog: "Bye"},
			"failFixt":         {SetUpError: "Whoa"},
			"tearDownFailFixt": {TearDownError: "Oops"},
			// Local fixtures can be accidentally included (crbug/1179162).
			"fixt1B": {},
		},
	}))
	defer td.Close()

	var gotRunArgs []*runner.RunnerRunTestsArgs

	td.RunFunc = func(args *runner.RunnerArgs, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunnerRunTestsMode:
			gotRunArgs = append(gotRunArgs, args.RunTests)

			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.RunnerListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.RunnerListFixturesResult{
				Fixtures: map[string][]*jsonprotocol.EntityInfo{
					"/path/to/cros": {
						&jsonprotocol.EntityInfo{Name: "fixt1B", Fixture: "remoteFixt"},
						&jsonprotocol.EntityInfo{Name: "fixt2", Fixture: "failFixt"},
						&jsonprotocol.EntityInfo{Name: "fixt3A", Fixture: "localFixt"},
						&jsonprotocol.EntityInfo{Name: "fixt3B"},
						&jsonprotocol.EntityInfo{Name: "localFixt"},
					},
				},
			})
		}
		return 0
	}
	td.Cfg.TestsToRun = []*resultsjson.Result{
		{Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "remoteFixt",
			Name:    "pkg.Test1A",
		}}, {Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "fixt1B", // depends on remoteFixt
			Name:    "pkg.Test1B",
		}}, {Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "fixt2", // depends on failFixt
			Name:    "pkg.Test2",
		}}, {Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "fixt3A", // depends on localFixt
			Name:    "pkg.Test3A",
		}}, {Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "fixt3B", // depends on nothing
			Name:    "pkg.Test3B",
		}}, {Test: resultsjson.Test{
			Bundle: "cros",
			Name:   "pkg.Test3C",
		}}, {Test: resultsjson.Test{
			Bundle:  "cros",
			Fixture: "tearDownFailFixt",
			Name:    "pkg.Test4",
		}},
		{
			// Remote tests should not be used on computing fixtures to run.
			BundleType: resultsjson.RemoteBundle,
			Test: resultsjson.Test{
				Bundle:  "cros",
				Fixture: "shouldNotRun",
				Name:    "pkg.RemoteTest",
			},
		},
	}

	cc := target.NewConnCache(&td.Cfg, td.Cfg.Target)
	defer cc.Close(context.Background())

	_, err := RunLocalTests(context.Background(), &td.Cfg, &td.State, cc)
	if err != nil {
		t.Fatalf("RunLocalTests(): %v", err)
	}

	// Test chunks are sorted by depending remote fixture name.
	want := []*runner.RunnerRunTestsArgs{
		{BundleArgs: jsonprotocol.BundleRunTestsArgs{
			Patterns: []string{"pkg.Test3A", "pkg.Test3B", "pkg.Test3C"},
		}}, {BundleArgs: jsonprotocol.BundleRunTestsArgs{
			Patterns:         []string{"pkg.Test2"},
			StartFixtureName: "failFixt",
			SetUpErrors:      []string{"Whoa"},
		}}, {BundleArgs: jsonprotocol.BundleRunTestsArgs{
			Patterns:         []string{"pkg.Test1A", "pkg.Test1B"},
			StartFixtureName: "remoteFixt",
		}}, {BundleArgs: jsonprotocol.BundleRunTestsArgs{
			Patterns:         []string{"pkg.Test4"},
			StartFixtureName: "tearDownFailFixt",
		}},
	}
	for _, w := range want {
		w.BundleGlob = fakerunner.MockLocalBundleGlob
		w.Devservers = fakerunner.MockDevservers
		w.BuildArtifactsURLDeprecated = fakerunner.MockBuildArtifactsURL

		w.BundleArgs.DataDir = fakerunner.MockLocalDataDir
		w.BundleArgs.OutDir = fakerunner.MockLocalOutDir
		w.BundleArgs.Devservers = fakerunner.MockDevservers
		w.BundleArgs.DUTName = td.Cfg.Target
		w.BundleArgs.BuildArtifactsURL = fakerunner.MockBuildArtifactsURL
		w.BundleArgs.DownloadMode = planner.DownloadLazy
		w.BundleArgs.HeartbeatInterval = heartbeatInterval
	}

	if diff := cmp.Diff(gotRunArgs, want); diff != "" {
		t.Errorf("RunnerArgs mismatch (-got +want):\n%v", diff)
	}
}
