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
	"chromiumos/tast/control"
	"chromiumos/tast/host"
	"chromiumos/tast/host/test"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

const (
	builtinBundleGlob = localBundleBuiltinDir + "/*"
)

// localTestData holds data shared between tests that exercise the local function.
type localTestData struct {
	srvData *test.TestData
	logbuf  bytes.Buffer
	cfg     Config
	tempDir string

	hostDir     string // directory simulating root dir on DUT for file copies
	nextCopyCmd string // next "exec" command expected for file copies

	expRunCmd string        // expected "exec" command for starting local_test_runner
	runStatus int           // status code for local_test_runner to return
	runStdout []byte        // stdout for local_test_runner to return
	runStderr []byte        // stderr for local_test_runner to return
	runStdin  bytes.Buffer  // stdin that was written to local_test_runner
	runDelay  time.Duration // local_test_runner delay before exiting
}

// newLocalTestData performs setup for tests that exercise the local function.
// It calls t.Fatal on error.
func newLocalTestData(t *gotesting.T) *localTestData {
	td := localTestData{expRunCmd: localRunnerPath}
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

	// Avoid checking test dependencies, which causes an extra local_test_runner call.
	td.cfg.checkTestDeps = checkTestDepsNever

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
	// TODO(derat): Remove this after 20180901: https://crbug.com/857485
	case "test -d " + host.QuoteShellArg(filepath.Join(localDataBuiltinDir, localBundlePkgPathPrefix)):
		req.Start(true)
		req.End(0)
	case td.expRunCmd:
		req.Start(true)
		io.Copy(&td.runStdin, req)
		req.Write(td.runStdout)
		req.Stderr().Write(td.runStderr)
		req.CloseOutput()
		time.Sleep(td.runDelay)
		req.End(td.runStatus)
	case td.nextCopyCmd:
		req.Start(true)
		req.End(req.RunRealCmd())
	default:
		log.Printf("Unexpected command %q", req.Cmd)
		req.Start(false)
	}
}

// checkArgs unmarshals a runner.Args struct from td.runStdin and compares it to exp.
func (td *localTestData) checkArgs(t *gotesting.T, exp *runner.Args) {
	args := runner.Args{}
	if err := json.NewDecoder(&td.runStdin).Decode(&args); err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(args, *exp) {
		t.Errorf("got args %+v; want %+v", args, *exp)
	}
}

func TestLocalSuccess(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
	td.runStdout = ob.Bytes()

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	td.checkArgs(t, &runner.Args{
		BundleGlob: builtinBundleGlob,
		DataDir:    localDataBuiltinDir,
	})
}

func TestLocalProxy(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
	td.runStdout = ob.Bytes()

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
		fmt.Sprintf("HTTP_PROXY=" + host.QuoteShellArg(httpProxy)),
		fmt.Sprintf("HTTPS_PROXY=" + host.QuoteShellArg(httpsProxy)),
		fmt.Sprintf("NO_PROXY=" + host.QuoteShellArg(noProxy)),
	}, " ") + " " + td.expRunCmd

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
}

func TestLocalExecFailure(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
	const stderr = "some failure message\n"
	td.runStatus = 1
	td.runStdout = ob.Bytes()
	td.runStderr = []byte(stderr)

	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v", status.ExitCode, subcommands.ExitFailure)
	}
	if !strings.Contains(td.logbuf.String(), stderr) {
		t.Errorf("local() logged %q; want substring %q", td.logbuf.String(), stderr)
	}
}

func TestLocalWaitTimeout(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Simulate local_test_runner writing control messages immediately but hanging before exiting.
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0)})
	td.runStdout = b.Bytes()
	td.runDelay = time.Minute

	// After setting a short wait timeout, an error should be reported.
	td.cfg.localRunnerWaitTimeout = time.Millisecond
	if status, _ := local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitFailure, td.logbuf.String())
	}
}

func TestLocalList(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	tests := []testing.Test{
		testing.Test{Name: "pkg.Test", Desc: "This is a test", Attr: []string{"attr1", "attr2"}},
		testing.Test{Name: "pkg.AnotherTest", Desc: "Another test"},
	}
	var err error
	if td.runStdout, err = json.Marshal(tests); err != nil {
		t.Fatal(err)
	}

	td.cfg.mode = ListTestsMode
	var status Status
	var results []TestResult
	if status, results = local(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	td.checkArgs(t, &runner.Args{
		Mode:       runner.ListTestsMode,
		BundleGlob: builtinBundleGlob,
		DataDir:    localDataBuiltinDir,
	})

	listed := make([]testing.Test, len(results))
	for i := 0; i < len(results); i++ {
		listed[i] = results[i].Test
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
		bundle      = "bnd"  // test bundle
		bundlePkg   = "chromiumos/tast/local/bundles/" + bundle
		category    = "cat" // test category
		categoryPkg = bundlePkg + "/" + category
		pattern     = "cat.*" // wildcard matching all tests

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
	tests := []testing.Test{
		testing.Test{Name: category + ".Test1", Pkg: categoryPkg, Data: []string{file1, file2}},
		testing.Test{Name: category + ".Test2", Pkg: categoryPkg, Data: []string{file2, file3, extFile1, extFile2}},
	}
	var err error
	if td.runStdout, err = json.Marshal(tests); err != nil {
		t.Fatal(err)
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
	if err = testutil.WriteFiles(filepath.Join(td.cfg.buildWorkspace, "src", tests[0].DataDir()), srcFiles); err != nil {
		t.Fatal(err)
	}

	// Prepare a fake destination directory.
	pushDir := filepath.Join(td.hostDir, localDataPushDir)
	dstFiles := map[string]string{
		extLinkFile2: extLinkFile2,
	}
	if err = testutil.WriteFiles(filepath.Join(pushDir, tests[0].DataDir()), dstFiles); err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal(err)
	}
	td.cfg.buildBundle = bundle
	td.cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.cfg, td.cfg.hst, builtinBundleGlob)
	if err != nil {
		t.Fatal("getDataFilePaths() failed: ", err)
	}
	td.checkArgs(t, &runner.Args{
		Mode:       runner.ListTestsMode,
		BundleGlob: builtinBundleGlob,
		Patterns:   []string{pattern},
	})
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
		filepath.Join(localDataPushDir, bundlePkg), paths); err != nil {
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

// TODO(derat): Add a test that verifies that getInitialSysInfo is called before tests are run.
