// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/fakerunner"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testutil"
)

// checkArgs compares two runner.Args.
func checkArgs(t *gotesting.T, args, exp *runner.Args) {
	t.Helper()
	if diff := cmp.Diff(args, exp, cmp.AllowUnexported(runner.Args{})); diff != "" {
		t.Errorf("Args mismatch (-got +want):\n%v", diff)
	}
}

func TestLocalSuccess(t *gotesting.T) {
	t.Parallel()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			checkArgs(t, args, &runner.Args{
				RunTests: &runner.RunTestsArgs{
					BundleArgs: bundle.RunTestsArgs{
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
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		}
		return 0
	}

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // avoid test being blocked indefinitely
	defer cancel()

	if _, err := runLocalTests(ctx, &td.Cfg, &td.State, cc); err != nil {
		t.Errorf("runLocalTest failed: %v", err)
	}
}

func TestLocalProxy(t *gotesting.T) {
	t.Parallel()

	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
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

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Error("runLocalTests failed: ", err)
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

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{testName}})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}, OutDir: filepath.Join(td.Cfg.LocalOutDir, outName)})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: td.Cfg.LocalOutDir})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		}
		return 0
	}

	if err := testutil.WriteFiles(filepath.Join(td.HostDir, td.Cfg.LocalOutDir), map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	td.Cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{
		Name: testName,
	}}}

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc); err != nil {
		t.Fatalf("runLocalTests failed: %v", err)
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

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		io.WriteString(stderr, msg)
		return 1
	}

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Error("runLocalTests unexpectedly passed")
	}
	if !strings.Contains(td.LogBuf.String(), msg) {
		t.Errorf("runLocalTests logged %q; want substring %q", td.LogBuf.String(), msg)
	}
}

func TestLocalWaitTimeout(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	// Simulate local_test_runner writing control messages immediately but hanging before exiting.
	td.RunDelay = time.Minute
	td.Cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{Name: "pkg.Foo"}}}
	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0)})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		}
		return 0
	}

	// After setting a short wait timeout, an error should be reported.
	td.Cfg.LocalRunnerWaitTimeout = time.Millisecond

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc); err == nil {
		t.Error("runLocalTests unexpectedly passed")
	}
}

func TestLocalDataFiles(t *gotesting.T) {
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
	)

	// Make local_test_runner list two tests containing the first three files (with overlap).
	tests := []testing.EntityInfo{
		{Name: category + ".Test1", Pkg: categoryPkg, Data: []string{file1, file2}},
		{Name: category + ".Test2", Pkg: categoryPkg, Data: []string{file2, file3, extFile1, extFile2}},
	}

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{
				BundleArgs: bundle.ListTestsArgs{Patterns: []string{pattern}},
				BundleGlob: fakerunner.MockLocalBundleGlob,
			},
		})

		json.NewEncoder(stdout).Encode(tests)
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

	// Prepare a fake destination directory.
	pushDir := filepath.Join(td.HostDir, fakerunner.MockLocalDataDir)
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(pushDir, testing.RelativeDataDir(tests[0].Pkg)), dstFiles); err != nil {
		t.Fatal(err)
	}

	// Connect to the target.
	cc := target.NewConnCache(&td.Cfg)
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
		filepath.Join(category, dataSubdir, file1),
		filepath.Join(category, dataSubdir, file2),
		filepath.Join(category, dataSubdir, file3),
		filepath.Join(category, dataSubdir, extFile1),
		filepath.Join(category, dataSubdir, extFile2),
	}
	if !reflect.DeepEqual(paths, expPaths) {
		t.Fatalf("getDataFilePaths() = %v; want %v", paths, expPaths)
	}

	// pushDataFiles should copy the required files to the DUT.
	if err = pushDataFiles(context.Background(), &td.Cfg, conn.SSHConn(),
		filepath.Join(fakerunner.MockLocalDataDir, bundlePkg), paths); err != nil {
		t.Fatal("pushDataFiles() failed: ", err)
	}
	expData := map[string]string{
		filepath.Join(testing.RelativeDataDir(tests[0].Pkg), file1):        file1,
		filepath.Join(testing.RelativeDataDir(tests[0].Pkg), file2):        file2,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), file3):        file3,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), extLinkFile1): extLinkFile1,
		filepath.Join(testing.RelativeDataDir(tests[1].Pkg), extFile2):     extFile2,
	}
	if data, err := testutil.ReadFiles(pushDir); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(data, expData) {
		t.Errorf("pushDataFiles() copied %v; want %v", data, expData)
	}
	if _, err := ioutil.ReadFile(filepath.Join(pushDir, testing.RelativeDataDir(tests[1].Pkg), extFile1)); err == nil {
		t.Errorf("pushDataFiles() unexpectedly copied %s", extFile1)
	}
}

// TestLocalMaxFailures makes sure that runLocalTests does not run any tests after maximum failures allowed has been reach.
func TestLocalMaxFailures(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t)
	defer td.Close()

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 2})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: "t1"}})
			mw.WriteMessage(&control.EntityError{Time: time.Unix(3, 0), Name: "t1", Error: testing.Error{Reason: "error"}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(4, 0), Name: "t1"})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(5, 0), Info: testing.EntityInfo{Name: "t2"}})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(6, 0), Name: "t2"})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(7, 0), OutDir: ""})
		case runner.ListFixturesMode:
			fmt.Fprintln(stdout, "{}") // no fixtures
		}
		return 0
	}
	td.Cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{Name: "pkg.Test"}}}
	td.Cfg.MaxTestFailures = 1
	td.State.FailuresCount = 0

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	results, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc)
	if err == nil {
		t.Errorf("runLocalTests() passed unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("runLocalTests return %v results; want 1", len(results))
	}
}

func TestFixturesDependency(t *gotesting.T) {
	td := fakerunner.NewLocalTestData(t, fakerunner.WithFakeRemoteRunnerData([]*testing.EntityInfo{
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

	var gotRunArgs []*runner.RunTestsArgs

	td.RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			gotRunArgs = append(gotRunArgs, args.RunTests)

			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{
				Fixtures: map[string][]*testing.EntityInfo{
					"/path/to/cros": {
						&testing.EntityInfo{Name: "fixt1B", Fixture: "remoteFixt"},
						&testing.EntityInfo{Name: "fixt2", Fixture: "failFixt"},
						&testing.EntityInfo{Name: "fixt3A", Fixture: "localFixt"},
						&testing.EntityInfo{Name: "fixt3B"},
						&testing.EntityInfo{Name: "localFixt"},
					},
				},
			})
		}
		return 0
	}
	td.Cfg.TestsToRun = []*jsonprotocol.EntityResult{
		{EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "remoteFixt",
			Name:    "pkg.Test1A",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "fixt1B", // depends on remoteFixt
			Name:    "pkg.Test1B",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "fixt2", // depends on failFixt
			Name:    "pkg.Test2",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "fixt3A", // depends on localFixt
			Name:    "pkg.Test3A",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "fixt3B", // depends on nothing
			Name:    "pkg.Test3B",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle: "cros",
			Name:   "pkg.Test3C",
		}}, {EntityInfo: testing.EntityInfo{
			Bundle:  "cros",
			Fixture: "tearDownFailFixt",
			Name:    "pkg.Test4",
		}},
	}

	cc := target.NewConnCache(&td.Cfg)
	defer cc.Close(context.Background())

	_, err := runLocalTests(context.Background(), &td.Cfg, &td.State, cc)
	if err != nil {
		t.Fatalf("runLocalTests(): %v", err)
	}

	// Test chunks are sorted by depending remote fixture name.
	want := []*runner.RunTestsArgs{
		{BundleArgs: bundle.RunTestsArgs{
			Patterns: []string{"pkg.Test3A", "pkg.Test3B", "pkg.Test3C"},
		}}, {BundleArgs: bundle.RunTestsArgs{
			Patterns:         []string{"pkg.Test2"},
			StartFixtureName: "failFixt",
			SetUpErrors:      []string{"Whoa"},
		}}, {BundleArgs: bundle.RunTestsArgs{
			Patterns:         []string{"pkg.Test1A", "pkg.Test1B"},
			StartFixtureName: "remoteFixt",
		}}, {BundleArgs: bundle.RunTestsArgs{
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
		t.Errorf("Args mismatch (-got +want):\n%v", diff)
	}
}
