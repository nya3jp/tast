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

	"chromiumos/tast/bundle"
	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = sshtest.MustGenerateKeys()
}

const (
	mockLocalRunner       = "/mock/local_test_runner"
	mockLocalBundleDir    = "/mock/local_bundles"
	mockLocalDataDir      = "/mock/local_data"
	mockLocalOutDir       = "/mock/local_out"
	mockBuildArtifactsURL = "gs://mock-images/artifacts/"

	mockLocalBundleGlob = mockLocalBundleDir + "/*"

	defaultBootID = "01234567-89ab-cdef-0123-456789abcdef"
)

var mockDevservers = []string{"192.168.0.1:12345", "192.168.0.2:23456"}

type runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int)

// localTestData holds data shared between tests that exercise the local function.
type localTestData struct {
	srvData *sshtest.TestData
	logbuf  bytes.Buffer
	cfg     Config
	tempDir string

	hostDir     string // directory simulating root dir on DUT for file copies
	nextCopyCmd string // next "exec" command expected for file copies

	bootID     string // simulated content of boot_id file
	unifiedLog string // simulated content of unified system log for the initial boot (defaultBootID)
	ramOops    string // simulated content of console-ramoops file

	expRunCmd string        // expected "exec" command for starting local_test_runner
	runFunc   runFunc       // called for expRunCmd executions
	runDelay  time.Duration // local_test_runner delay before exiting
}

// newLocalTestData performs setup for tests that exercise the local function.
// It calls t.Fatal on error.
func newLocalTestData(t *gotesting.T) *localTestData {
	td := localTestData{expRunCmd: "exec env " + mockLocalRunner, bootID: defaultBootID}
	td.srvData = sshtest.NewTestData(userKey, hostKey, td.handleExec)
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
	td.cfg.devservers = mockDevservers
	td.cfg.buildArtifactsURL = mockBuildArtifactsURL
	td.cfg.downloadMode = planner.DownloadLazy
	td.cfg.totalShards = 1
	td.cfg.shardIndex = 0

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
func (td *localTestData) handleExec(req *sshtest.ExecReq) {
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
	case "exec sync":
		req.Start(true)
		req.End(0)
	case "exec croslog --quiet --boot=" + strings.Replace(defaultBootID, "-", "", -1) + " --lines=1000":
		req.Start(true)
		io.WriteString(req, td.unifiedLog)
		req.End(0)
	case "exec cat /sys/fs/pstore/console-ramoops", "exec cat /sys/fs/pstore/console-ramoops-0":
		req.Start(true)
		io.WriteString(req, td.ramOops)
		req.End(0)
	case "exec mkdir -p " + td.cfg.localOutDir:
		req.Start(true)
		req.End(0)
	default:
		req.Start(true)
		req.End(req.RunRealCmd())
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
func errorCounts(rs []*EntityResult) map[string]int {
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
					Devservers:        mockDevservers,
					DUTName:           td.cfg.Target,
					BuildArtifactsURL: mockBuildArtifactsURL,
					DownloadMode:      planner.DownloadLazy,
					HeartbeatInterval: heartbeatInterval,
				},
				BundleGlob:                  mockLocalBundleGlob,
				Devservers:                  mockDevservers,
				BuildArtifactsURLDeprecated: mockBuildArtifactsURL,
			},
		})

		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		return 0
	}

	if _, err := runLocalTests(context.Background(), &td.cfg); err != nil {
		t.Error("runLocalTest failed: ", err)
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

	if _, err := runLocalTests(context.Background(), &td.cfg); err != nil {
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

	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{testName}})
		mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}, OutDir: filepath.Join(td.cfg.localOutDir, outName)})
		mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: td.cfg.localOutDir})
		return 0
	}

	if err := testutil.WriteFiles(filepath.Join(td.hostDir, td.cfg.localOutDir), map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := runLocalTests(context.Background(), &td.cfg); err != nil {
		t.Error("runLocalTests failed: ", err)
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

	if _, err := runLocalTests(context.Background(), &td.cfg); err == nil {
		t.Error("runLocalTests unexpectedly passed")
	}
	if !strings.Contains(td.logbuf.String(), msg) {
		t.Errorf("runLocalTests logged %q; want substring %q", td.logbuf.String(), msg)
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
	if _, err := runLocalTests(context.Background(), &td.cfg); err == nil {
		t.Error("runLocalTests unexpectedly passed")
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
	tests := []testing.EntityInfo{
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
	if err := testutil.WriteFiles(filepath.Join(td.cfg.buildWorkspace, "src", testing.RelativeDataDir(tests[0].Pkg)), srcFiles); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	pushDir := filepath.Join(td.hostDir, mockLocalDataDir)
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(pushDir, testing.RelativeDataDir(tests[0].Pkg)), dstFiles); err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal(err)
	}
	td.cfg.buildBundle = bundleName
	td.cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.cfg, td.cfg.hst)
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
