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
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/subcommands"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/bundle"
	"chromiumos/tast/control"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
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
	args   runner.Args  // args that were passed to fake runner
}

// newRemoteTestData creates a temporary directory with a symlink back to the unit test binary
// that's currently running. It also writes a config file instructing the test binary about
// its stdout, stderr, and status code when running as a fake runner.
func newRemoteTestData(t *gotesting.T, stdout, stderr string, status int) *remoteTestData {
	exec, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	td := remoteTestData{}
	td.dir = testutil.TempDir(t)
	td.cfg.Logger = logging.NewSimple(&td.logbuf, log.LstdFlags, true)
	td.cfg.ResDir = filepath.Join(td.dir, "results")
	if err = os.MkdirAll(td.cfg.ResDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg.localBundleDir = mockLocalBundleDir
	td.cfg.remoteOutDir = filepath.Join(td.cfg.ResDir, "out.tmp")

	// Create a symlink to ourselves that can be executed as a fake test runner.
	td.cfg.remoteRunner = filepath.Join(td.dir, fakeRunnerName)
	if err = os.Symlink(exec, td.cfg.remoteRunner); err != nil {
		os.RemoveAll(td.dir)
		t.Fatal(err)
	}

	// Write a config file telling the fake runner what to do.
	rcfg := fakeRunnerConfig{Stdout: stdout, Stderr: stderr, Status: status}
	f, err := os.Create(filepath.Join(td.dir, fakeRunnerConfigFile))
	if err != nil {
		os.RemoveAll(td.dir)
		t.Fatal(err)
	}
	defer f.Close()
	if err = json.NewEncoder(f).Encode(&rcfg); err != nil {
		os.RemoveAll(td.dir)
		t.Fatal(err)
	}

	return &td
}

// close removes the temporary directory.
func (td *remoteTestData) close() {
	td.cfg.Close(context.Background())
	os.RemoveAll(td.dir)
}

// run calls remote and records the Args struct that was passed to the fake runner.
func (td *remoteTestData) run(t *gotesting.T) (subcommands.ExitStatus, []TestResult) {
	status, res := remote(context.Background(), &td.cfg)

	f, err := os.Open(filepath.Join(td.dir, fakeRunnerArgsFile))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err = json.NewDecoder(f).Decode(&td.args); err != nil {
		t.Fatal(err)
	}

	return status.ExitCode, res
}

func TestRemoteRun(t *gotesting.T) {
	const testName = "pkg.Test"

	b := bytes.Buffer{}
	tm := time.Unix(1, 0)
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: tm, NumTests: 1})
	mw.WriteMessage(&control.TestStart{Time: tm, Test: testing.TestCase{Name: testName}})
	mw.WriteMessage(&control.TestEnd{Time: tm, Name: testName})
	mw.WriteMessage(&control.RunEnd{Time: tm, OutDir: ""})

	td := newRemoteTestData(t, b.String(), "", 0)
	defer td.close()

	// Set some parameters that can be overridden by flags to arbitrary values.
	td.cfg.KeyFile = "/tmp/id_dsa"
	td.cfg.remoteBundleDir = "/tmp/bundles"
	td.cfg.remoteDataDir = "/tmp/data"
	td.cfg.remoteOutDir = "/tmp/out"

	status, res := td.run(t)
	if status != subcommands.ExitSuccess {
		t.Errorf("remote(%+v) returned status %v; want %v", td.cfg, status, subcommands.ExitSuccess)
	}
	if !td.cfg.startedRun {
		t.Error("remote() incorrectly reported that run wasn't started")
	}
	if len(res) != 1 {
		t.Errorf("remote(%+v) returned %v result(s); want 1", td.cfg, len(res))
	} else if res[0].Name != testName {
		t.Errorf("remote(%+v) returned result for test %q; want %q", td.cfg, res[0].Name, testName)
	}

	glob := filepath.Join(td.cfg.remoteBundleDir, "*")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runFlags := []string{
		"-keyfile=" + td.cfg.KeyFile,
		"-keydir=",
		"-remoterunner=" + td.cfg.remoteRunner,
		"-remotebundledir=" + td.cfg.remoteBundleDir,
		"-remotedatadir=" + td.cfg.remoteDataDir,
	}
	expArgs := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleGlob: glob,
			BundleArgs: bundle.RunTestsArgs{
				DataDir:           td.cfg.remoteDataDir,
				OutDir:            td.cfg.remoteOutDir,
				KeyFile:           td.cfg.KeyFile,
				TastPath:          exe,
				RunFlags:          runFlags,
				LocalBundleDir:    mockLocalBundleDir,
				CheckSoftwareDeps: false,
				HeartbeatInterval: heartbeatInterval,
			},
		},
	}
	if !reflect.DeepEqual(td.args, expArgs) {
		t.Errorf("remote(%+v) passed args %+v; want %+v", td.cfg, td.args, expArgs)
	}
}

func TestRemoteList(t *gotesting.T) {
	// Make the runner print serialized tests.
	tests := []testing.TestCase{
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
	td.cfg.mode = ListTestsMode
	td.cfg.remoteDataDir = "/tmp/data"
	td.cfg.Patterns = []string{"*Test*"}

	var status subcommands.ExitStatus
	var results []TestResult
	if status, results = td.run(t); status != subcommands.ExitSuccess {
		t.Errorf("remote(%v) returned status %v; want %v", td.cfg, status, subcommands.ExitSuccess)
	}
	glob := filepath.Join(td.cfg.remoteBundleDir, "*")
	expArgs := runner.Args{
		Mode: runner.ListTestsMode,
		ListTests: &runner.ListTestsArgs{
			BundleArgs: bundle.ListTestsArgs{Patterns: td.cfg.Patterns},
			BundleGlob: glob,
		},
	}
	if !reflect.DeepEqual(td.args, expArgs) {
		t.Errorf("remote(%+v) passed args %+v; want %+v", td.cfg, td.args, expArgs)
	}

	listed := make([]testing.TestCase, len(results))
	for i := 0; i < len(results); i++ {
		listed[i] = results[i].TestCase
	}
	if !reflect.DeepEqual(listed, tests) {
		t.Errorf("remote() listed tests %+v; want %+v", listed, tests)
	}
}

func TestRemoteFailure(t *gotesting.T) {
	// Make the test runner print a message to stderr and fail.
	const errorMsg = "Whoops, something failed\n"
	td := newRemoteTestData(t, "", errorMsg, 1)
	defer td.close()

	if status, _ := td.run(t); status != subcommands.ExitFailure {
		t.Errorf("remote(%v) returned status %v; want %v", td.cfg, status, subcommands.ExitFailure)
	}
	// The runner's error message should've been logged.
	if !strings.Contains(td.logbuf.String(), errorMsg) {
		t.Errorf("remote(%v) didn't log runner error %q in %q", td.cfg, errorMsg, td.logbuf.String())
	}
	if !td.cfg.startedRun {
		t.Error("remote() incorrectly reported that run wasn't started")
	}
}

// TODO(derat): Add a test that verifies that getInitialSysInfo is called before tests are run.
// Also verify that cfg.startedRun is false if we see an early failure during getInitialSysInfo.
