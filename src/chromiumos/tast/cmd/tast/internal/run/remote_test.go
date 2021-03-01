// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

const (
	fakeRunnerName       = "fake_runner"             // symlink to this executable created by newRemoteTestData
	fakeRunnerConfigFile = "fake_runner_config.json" // config file read when acting as fake runner
	fakeRunnerArgsFile   = "fake_runner_args.json"   // file containing args written when acting as fake runner
)

func init() {
	// If the binary was executed via a symlink created by newRemoteTestData,
	// behave like a test runner instead of running unit tests.
	if filepath.Base(os.Args[0]) == fakeRunnerName {
		os.Exit(runFakeRunner())
	}
}

// fakeRunnerConfig describes this executable's output when it's acting as a fake test runner.
type fakeRunnerConfig struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Status int    `json:"status"`
}

// runFakeRunner saves stdin to the current directory, reads a config, and writes the requested
// data to stdout and stderr. It returns the status code to exit with.
func runFakeRunner() int {
	dir := filepath.Dir(os.Args[0])

	// Write the arguments we received.
	af, err := os.Create(filepath.Join(dir, fakeRunnerArgsFile))
	if err != nil {
		log.Fatal(err)
	}
	defer af.Close()
	if _, err = io.Copy(af, os.Stdin); err != nil {
		log.Fatal(err)
	}

	if err = json.NewEncoder(af).Encode(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	// Read our configuration.
	cf, err := os.Open(filepath.Join(dir, fakeRunnerConfigFile))
	if err != nil {
		log.Fatal(err)
	}
	defer cf.Close()

	cfg := fakeRunnerConfig{}
	if err = json.NewDecoder(cf).Decode(&cfg); err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write([]byte(cfg.Stdout))
	os.Stderr.Write([]byte(cfg.Stderr))
	return cfg.Status
}

// remoteTestData holds data corresponding to the current unit test.
type remoteTestData struct {
	dir    string       // temp dir
	logbuf bytes.Buffer // logging output
	cfg    Config       // config passed to remote
	state  State        // state passed to remote
	args   runner.Args  // args that were passed to fake runner
}

type fakeRemoteRunnerData struct {
	stdout string
	stderr string
	status int
}

func (rd *fakeRemoteRunnerData) setUp(dir string) (string, error) {
	exec, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Create a symlink to ourselves that can be executed as a fake test runner.
	runner := filepath.Join(dir, fakeRunnerName)
	if err = os.Symlink(exec, runner); err != nil {
		return "", err
	}

	// Write a config file telling the fake runner what to do.
	rcfg := fakeRunnerConfig{Stdout: rd.stdout, Stderr: rd.stderr, Status: rd.status}
	f, err := os.Create(filepath.Join(dir, fakeRunnerConfigFile))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = json.NewEncoder(f).Encode(&rcfg); err != nil {
		return "", err
	}
	return runner, nil
}

// newRemoteTestData creates a temporary directory with a symlink back to the unit test binary
// that's currently running. It also writes a config file instructing the test binary about
// its stdout, stderr, and status code when running as a fake runner.
func newRemoteTestData(t *gotesting.T, stdout, stderr string, status int) *remoteTestData {
	td := remoteTestData{}
	td.dir = testutil.TempDir(t)
	td.cfg.Logger = logging.NewSimple(&td.logbuf, true, true)
	td.cfg.ResDir = filepath.Join(td.dir, "results")
	if err := os.MkdirAll(td.cfg.ResDir, 0755); err != nil {
		os.RemoveAll(td.dir)
		t.Fatal(err)
	}
	td.cfg.Devservers = mockDevservers
	td.cfg.BuildArtifactsURL = mockBuildArtifactsURL
	td.cfg.DownloadMode = planner.DownloadLazy
	td.cfg.LocalRunner = mockLocalRunner
	td.cfg.LocalBundleDir = mockLocalBundleDir
	td.cfg.LocalDataDir = mockLocalDataDir
	td.cfg.RemoteOutDir = filepath.Join(td.cfg.ResDir, "out.tmp")
	td.cfg.TotalShards = 1
	td.cfg.ShardIndex = 0

	path, err := (&fakeRemoteRunnerData{stdout, stderr, status}).setUp(td.dir)
	if err != nil {
		os.RemoveAll(td.dir)
		t.Fatal(err)
	}
	td.cfg.RemoteRunner = path

	td.state.RemoteDevservers = td.cfg.Devservers

	return &td
}

// close removes the temporary directory.
func (td *remoteTestData) close() {
	td.state.Close(context.Background())
	os.RemoveAll(td.dir)
}

// run calls remote and records the Args struct that was passed to the fake runner.
func (td *remoteTestData) run(t *gotesting.T) ([]*EntityResult, error) {
	res, rerr := runRemoteTests(context.Background(), &td.cfg, &td.state)

	f, err := os.Open(filepath.Join(td.dir, fakeRunnerArgsFile))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err = json.NewDecoder(f).Decode(&td.args); err != nil {
		t.Fatal(err)
	}

	return res, rerr
}

func TestRemoteRun(t *gotesting.T) {
	const testName = "pkg.Test"

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""})

	td := newRemoteTestData(t, b.String(), "", 0)
	defer td.close()

	// Set some parameters that can be overridden by flags to arbitrary values.
	td.cfg.Build = false
	td.cfg.KeyFile = "/tmp/id_dsa"
	td.cfg.RemoteBundleDir = "/tmp/bundles"
	td.cfg.RemoteDataDir = "/tmp/data"
	td.cfg.RemoteOutDir = "/tmp/out"

	res, err := td.run(t)
	if err != nil {
		t.Errorf("runRemoteTests(%+v) failed: %v", td.cfg, err)
	}
	if len(res) != 1 {
		t.Errorf("runRemoteTests(%+v) returned %v result(s); want 1", td.cfg, len(res))
	} else if res[0].Name != testName {
		t.Errorf("runRemoteTests(%+v) returned result for test %q; want %q", td.cfg, res[0].Name, testName)
	}

	glob := filepath.Join(td.cfg.RemoteBundleDir, "*")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runFlags := []string{
		"-build=false", // propagated from td.cfg.build
		"-keyfile=" + td.cfg.KeyFile,
		"-keydir=",
		"-remoterunner=" + td.cfg.RemoteRunner,
		"-remotebundledir=" + td.cfg.RemoteBundleDir,
		"-remotedatadir=" + td.cfg.RemoteDataDir,
		"-localrunner=" + td.cfg.LocalRunner,
		"-localbundledir=" + td.cfg.LocalBundleDir,
		"-localdatadir=" + td.cfg.LocalDataDir,
		"-devservers=" + strings.Join(td.cfg.Devservers, ","),
	}
	expArgs := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleGlob: glob,
			BundleArgs: bundle.RunTestsArgs{
				DataDir:        td.cfg.RemoteDataDir,
				OutDir:         td.cfg.RemoteOutDir,
				KeyFile:        td.cfg.KeyFile,
				TastPath:       exe,
				RunFlags:       runFlags,
				LocalBundleDir: mockLocalBundleDir,
				FeatureArgs: bundle.FeatureArgs{
					CheckSoftwareDeps: false,
				},
				Devservers:        mockDevservers,
				DownloadMode:      planner.DownloadLazy,
				HeartbeatInterval: heartbeatInterval,
			},
		},
	}
	if diff := cmp.Diff(td.args, expArgs, cmpopts.IgnoreUnexported(expArgs)); diff != "" {
		t.Errorf("runRemoteTests(%+v) passed unexpected args; diff (-got +want):\n%s", td.cfg, diff)
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
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}, OutDir: filepath.Join(outDir, outName)})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: outDir})

	td := newRemoteTestData(t, b.String(), "", 0)
	defer td.close()

	// Set some parameters that can be overridden by flags to arbitrary values.
	td.cfg.KeyFile = "/tmp/id_dsa"
	td.cfg.RemoteBundleDir = "/tmp/bundles"
	td.cfg.RemoteDataDir = "/tmp/data"
	td.cfg.RemoteOutDir = outDir

	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := td.run(t); err != nil {
		t.Errorf("runRemoteTests(%+v) failed: %v", td.cfg, err)
	}

	files, err := testutil.ReadFiles(filepath.Join(td.cfg.ResDir, testLogsDir))
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
	td := newRemoteTestData(t, "", errorMsg, 1)
	defer td.close()

	if _, err := td.run(t); err == nil {
		t.Errorf("runRemoteTests(%v) unexpectedly passed", td.cfg)
	} else if !strings.Contains(err.Error(), strings.TrimRight(errorMsg, "\n")) {
		// The runner's error message should've been logged.
		t.Errorf("runRemoteTests(%v) didn't log runner error %q in %q", td.cfg, errorMsg, err.Error())
	}
}

// TestRemoteMaxFailures makes sure that runRemoteTests does not run any tests if maximum failures allowed has been reach.
func TestRemoteMaxFailures(t *gotesting.T) {
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 2})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: "t1"}})
	mw.WriteMessage(&control.EntityError{Time: time.Unix(3, 0), Name: "t1", Error: testing.Error{Reason: "error"}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(4, 0), Name: "t1"})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(5, 0), Info: testing.EntityInfo{Name: "t2"}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(6, 0), Name: "t2"})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(7, 0), OutDir: ""})

	td := newRemoteTestData(t, b.String(), "", 0)
	defer td.close()

	td.cfg.MaxTestFailures = 1
	td.state.FailuresCount = 0

	results, err := runRemoteTests(context.Background(), &td.cfg, &td.state)
	if err == nil {
		t.Errorf("runRemoteTests(%+v, %+v) passed unexpectedly", td.cfg, td.state)
	}
	if len(results) != 1 {
		t.Errorf("runRemoteTests return %v results; want 1", len(results))
	}
}

// TODO(derat): Add a test that verifies that getInitialSysInfo is called before tests are run.
// Also verify that state.StartedRun is false if we see an early failure during getInitialSysInfo.
