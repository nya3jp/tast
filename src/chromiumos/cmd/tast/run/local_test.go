// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	"chromiumos/tast/command"
	"chromiumos/tast/control"
	"chromiumos/tast/host/test"
	"chromiumos/tast/runner"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

const (
	mockLocalRunner    = "/mock/local_test_runner"
	mockLocalBundleDir = "/mock/local_bundles"
	mockLocalDataDir   = "/mock/local_data"
	mockLocalOutDir    = "/mock/local_out"

	mockLocalBundleGlob = mockLocalBundleDir + "/*"

	defaultBootID = "01234567-89ab-cdef-0123-456789abcdef"
)

type runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int)

// localTestData holds data shared between tests that exercise the local function.
type localTestData struct {
	srvData *test.TestData
	logbuf  bytes.Buffer
	cfg     Config
	tempDir string

	hostDir     string // directory simulating root dir on DUT for file copies
	nextCopyCmd string // next "exec" command expected for file copies

	bootID  string // simulated content of boot_id file
	journal string // simulated content of systemd journal for the initial boot (defaultBootID)
	ramOops string // simulated content of console-ramoops file

	expRunCmd string        // expected "exec" command for starting local_test_runner
	runFunc   runFunc       // called for expRunCmd executions
	runDelay  time.Duration // local_test_runner delay before exiting
}

// newLocalTestData performs setup for tests that exercise the local function.
// It calls t.Fatal on error.
func newLocalTestData(t *gotesting.T) *localTestData {
	td := localTestData{expRunCmd: "exec env " + mockLocalRunner, bootID: defaultBootID}
	td.srvData = test.NewTestData(userKey, hostKey, td.handleExec)
	td.cfg.KeyFile = td.srvData.UserKeyFile

	toClose := &td
	defer func() { toClose.close() }()

	td.tempDir = testutil.TempDir(t)
	td.cfg.ResDir = filepath.Join(td.tempDir, "results")
	if err := os.Mkdir(td.cfg.ResDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg.Logger = logging.NewSimple(&td.logbuf, log.LstdFlags, true)
	td.cfg.Target = td.srvData.Srv.Addr().String()
	td.cfg.localRunner = mockLocalRunner
	td.cfg.localBundleDir = mockLocalBundleDir
	td.cfg.localDataDir = mockLocalDataDir
	td.cfg.localOutDir = mockLocalOutDir

	// Avoid checking test dependencies, which causes an extra local_test_runner call.
	td.cfg.checkTestDeps = false

	// Ensure that already-set environment variables don't affect unit tests.
	td.cfg.proxy = proxyNone

	// Run actual commands when performing file copies.
	td.hostDir = filepath.Join(td.tempDir, "host")
	if err := os.Mkdir(td.hostDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg.hstCopyBasePath = td.hostDir
	td.cfg.hstCopyAnnounceCmd = func(cmd string) { td.nextCopyCmd = cmd }

	toClose = nil
	return &td
}

func (td *localTestData) close() {
	if td == nil {
		return
	}
	td.cfg.Close(context.Background())
	td.srvData.Close()
	if td.tempDir != "" {
		os.RemoveAll(td.tempDir)
	}
}

// handleExec handles SSH "exec" requests sent to td.srvData.Srv.
// Canned results are returned for local_test_runner, while file-copying-related commands are actually executed.
func (td *localTestData) handleExec(req *test.ExecReq) {
	defer func() { td.nextCopyCmd = "" }()

	switch req.Cmd {
	case "exec cat /proc/sys/kernel/random/boot_id":
		req.Start(true)
		fmt.Fprintln(req, td.bootID)
		req.End(0)
	case td.expRunCmd:
		req.Start(true)
		var args runner.Args
		var status int
		if err := json.NewDecoder(req).Decode(&args); err != nil {
			status = command.WriteError(req.Stderr(), err)
		} else {
			status = td.runFunc(&args, req, req.Stderr())
		}
		req.CloseOutput()
		time.Sleep(td.runDelay)
		req.End(status)
	case td.nextCopyCmd:
		req.Start(true)
		req.End(req.RunRealCmd())
	case "exec sync":
		req.Start(true)
		req.End(0)
	case "exec journalctl -q -b " + strings.Replace(defaultBootID, "-", "", -1) + " -n 1000":
		req.Start(true)
		io.WriteString(req, td.journal)
		req.End(0)
	case "exec cat /sys/fs/pstore/console-ramoops", "exec cat /sys/fs/pstore/console-ramoops-0":
		req.Start(true)
		io.WriteString(req, td.ramOops)
		req.End(0)
	case "exec mkdir -p " + td.cfg.localOutDir:
		req.Start(true)
		req.End(0)
	default:
		log.Printf("Unexpected command %q", req.Cmd)
		req.Start(false)
	}
}

// checkArgs compares two runner.Args.
func checkArgs(t *gotesting.T, args, exp *runner.Args) {
	if !reflect.DeepEqual(args, exp) {
		t.Errorf("got args %+v; want %+v", *args, *exp)
	}
}

// errorCounts returns a map from test names in rs to the number of errors reported by each.
// This is useful for tests that just want to quickly check the results of a test run.
// Detailed tests for result generation are in results_test.go.
func errorCounts(rs []TestResult) map[string]int {
	testErrs := make(map[string]int)
	for _, r := range rs {
		testErrs[r.Name] = len(r.Errors)
	}
	return testErrs
}

func TestLocalSuccess(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			RunTests: &runner.RunTestsArgs{
				BundleArgs: bundle.RunTestsArgs{
					DataDir:           mockLocalDataDir,
					OutDir:            mockLocalOutDir,
					HeartbeatInterval: heartbeatInterval,
				},
				BundleGlob: mockLocalBundleGlob,
			},
		})

		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		return 0
	}

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if !td.cfg.startedRun {
		t.Error("local() incorrectly reported that run wasn't started")
	}
}

func TestLocalProxy(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
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
	td.cfg.proxy = proxyEnv

	// Proxy environment variables should be prepended to the local_test_runner command line.
	// (The variables are added in this order in local.go.)
	td.expRunCmd = strings.Join([]string{
		"exec",
		"env",
		shutil.Escape("HTTP_PROXY=" + httpProxy),
		shutil.Escape("HTTPS_PROXY=" + httpsProxy),
		shutil.Escape("NO_PROXY=" + noProxy),
		mockLocalRunner,
	}, " ")

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
}

func TestLocalCopyOutput(t *gotesting.T) {
	const (
		testName = "pkg.Test"
		outFile  = "somefile.txt"
		outData  = "somedata"
	)

	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{testName}})
		mw.WriteMessage(&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestCase{Name: testName}})
		mw.WriteMessage(&control.TestEnd{Time: time.Unix(3, 0), Name: testName})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: td.cfg.localOutDir})
		return 0
	}

	if err := testutil.WriteFiles(filepath.Join(td.hostDir, td.cfg.localOutDir), map[string]string{
		filepath.Join(testName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
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

func disabledTestLocalExecFailure(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	const msg = "some failure message\n"

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		io.WriteString(stderr, msg)
		return 1
	}

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v", status.ExitCode, subcommands.ExitFailure)
	}
	if !strings.Contains(td.logbuf.String(), msg) {
		t.Errorf("local() logged %q; want substring %q", td.logbuf.String(), msg)
	}
	if !td.cfg.startedRun {
		t.Error("local() incorrectly reported that run wasn't started")
	}
}

func TestLocalWaitTimeout(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Simulate local_test_runner writing control messages immediately but hanging before exiting.
	td.runDelay = time.Minute
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0)})
		return 0
	}

	// After setting a short wait timeout, an error should be reported.
	td.cfg.localRunnerWaitTimeout = time.Millisecond
	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitFailure, td.logbuf.String())
	}
	if !td.cfg.startedRun {
		t.Error("local() incorrectly reported that run wasn't started")
	}
}

func TestLocalList(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.TestCase{
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

	td.cfg.mode = ListTestsMode
	var status Status
	var results []TestResult
	if status, results = local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}

	listed := make([]testing.TestCase, len(results))
	for i := 0; i < len(results); i++ {
		listed[i] = results[i].TestCase
	}
	if !reflect.DeepEqual(listed, tests) {
		t.Errorf("local() listed tests %+v; want %+v", listed, tests)
	}
}

func TestLocalDataFiles(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

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
	tests := []testing.TestCase{
		{Name: category + ".Test1", Pkg: categoryPkg, Data: []string{file1, file2}},
		{Name: category + ".Test2", Pkg: categoryPkg, Data: []string{file2, file3, extFile1, extFile2}},
	}

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode: runner.ListTestsMode,
			ListTests: &runner.ListTestsArgs{
				BundleArgs: bundle.ListTestsArgs{Patterns: []string{pattern}},
				BundleGlob: mockLocalBundleGlob,
			},
		})

		json.NewEncoder(stdout).Encode(tests)
		return 0
	}

	// Create a fake source checkout and write the data files to it. Just use their names as their contents.
	td.cfg.buildWorkspace = filepath.Join(td.tempDir, "ws")
	srcFiles := map[string]string{
		file1:        file1,
		file2:        file2,
		file3:        file3,
		file4:        file4,
		extLinkFile1: extLinkFile1,
		extFile2:     extFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(td.cfg.buildWorkspace, "src", tests[0].DataDir()), srcFiles); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	pushDir := filepath.Join(td.hostDir, mockLocalDataDir)
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(pushDir, tests[0].DataDir()), dstFiles); err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal(err)
	}
	td.cfg.buildBundle = bundleName
	td.cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.cfg, td.cfg.hst, mockLocalBundleGlob)
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
	if err = pushDataFiles(context.Background(), &td.cfg, td.cfg.hst,
		filepath.Join(mockLocalDataDir, bundlePkg), paths); err != nil {
		t.Fatal("pushDataFiles() failed: ", err)
	}
	expData := map[string]string{
		filepath.Join(tests[0].DataDir(), file1):        file1,
		filepath.Join(tests[0].DataDir(), file2):        file2,
		filepath.Join(tests[1].DataDir(), file3):        file3,
		filepath.Join(tests[1].DataDir(), extLinkFile1): extLinkFile1,
		filepath.Join(tests[1].DataDir(), extFile2):     extFile2,
	}
	if data, err := testutil.ReadFiles(pushDir); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(data, expData) {
		t.Errorf("pushDataFiles() copied %v; want %v", data, expData)
	}
	if _, err := ioutil.ReadFile(filepath.Join(pushDir, tests[1].DataDir(), extFile1)); err == nil {
		t.Errorf("pushDataFiles() unexpectedly copied %s", extFile1)
	}
}

func TestLocalFailureBeforeRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the runner always fail, and ask to check test deps so we'll get a failure before trying
	// to run tests. local() shouldn't set startedRun to true since we failed before then.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }
	td.cfg.checkTestDeps = true
	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v", status.ExitCode, subcommands.ExitFailure)
	} else if td.cfg.startedRun {
		t.Error("local() incorrectly reported that run was started after early failure")
	}
}

func TestLocalContinueAfterFailure(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	const (
		test1 = "pkg.Test1"
		test2 = "pkg.Test2"
		test3 = "pkg.Test3"
		glob  = "pkg.*"
	)

	numCalls := 0 // number of RunTests calls
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		if args.Mode != runner.RunTestsMode {
			t.Errorf("Unexpected non-RunTests args %+v", args)
			return 1
		}

		numCalls++
		switch numCalls {
		case 1:
			// The first time, report that all three tests will be run but abort after starting #1.
			if exp := []string{glob}; !reflect.DeepEqual(args.RunTests.BundleArgs.Patterns, exp) {
				t.Errorf("Call %d had patterns %v; want %v", numCalls, args.RunTests.BundleArgs.Patterns, exp)
			}
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{test1, test2, test3}})
			mw.WriteMessage(&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestCase{Name: test1}})
			return 1
		case 2:
			// The second time, list the remaining two tests, run test #2 successfully, and abort after #3.
			if exp := []string{test2, test3}; !reflect.DeepEqual(args.RunTests.BundleArgs.Patterns, exp) {
				t.Errorf("Call %d had patterns %v; want %v", numCalls, args.RunTests.BundleArgs.Patterns, exp)
			}
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(3, 0), TestNames: []string{test2, test3}})
			mw.WriteMessage(&control.TestStart{Time: time.Unix(4, 0), Test: testing.TestCase{Name: test2}})
			mw.WriteMessage(&control.TestEnd{Time: time.Unix(5, 0), Name: test2})
			mw.WriteMessage(&control.TestStart{Time: time.Unix(6, 0), Test: testing.TestCase{Name: test3}})
			return 1
		default:
			// There aren't any more tests to run.
			t.Errorf("Unexpected RunTests call: %+v", args)
			return 1
		}
	}

	td.cfg.Patterns = []string{glob}
	td.cfg.continueAfterFailure = true
	status, results := local(context.Background(), &td.cfg)
	// Success should be reported since the bundle tried to execute all tests.
	if status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if numCalls != 2 {
		t.Errorf("Got %d RunTests call(s); want 2", numCalls)
	}
	testErrs := errorCounts(results)
	if exp := map[string]int{test1: 1, test2: 0, test3: 1}; !reflect.DeepEqual(testErrs, exp) {
		t.Errorf("Got error counts %v; want %v", testErrs, exp)
	}
}

func TestLocalContinueAfterFailureNoTests(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the runner fail without writing any control messages.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }

	// The run should be reported as having failed.
	td.cfg.continueAfterFailure = true
	status, results := local(context.Background(), &td.cfg)
	if status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitFailure, td.logbuf.String())
	}
	if len(results) != 0 {
		t.Errorf("Got result(s) %+v; want none", results)
	}
}

func TestLocalGetSoftwareFeatures(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.GetSoftwareFeaturesMode:
			// Just check that getSoftwareFeatures is called; details of args are
			// tested in deps_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetSoftwareFeaturesResult{
				Available: []string{"foo"}, // must report non-empty features
			})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.checkTestDeps = true

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if !called {
		t.Errorf("local did not call getSoftwareFeatures")
	}
}

func TestLocalGetInitialSysInfo(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	called := false

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.GetSysInfoStateMode:
			// Just check that getInitialSysInfo is called; details of args are
			// tested in sys_info_test.go.
			called = true
			json.NewEncoder(stdout).Encode(&runner.GetSysInfoStateResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.collectSysInfo = true

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if !called {
		t.Errorf("local did not call getInitialSysInfo")
	}
}
