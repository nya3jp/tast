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

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/control"
	"chromiumos/tast/host"
	"chromiumos/tast/host/test"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"

	"github.com/google/subcommands"
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
	hostDir string // directory simulating root dir on DUT for file copies
}

// newLocalTestData performs setup for tests that exercise the local function.
// Panics on error.
func newLocalTestData() *localTestData {
	td := localTestData{srvData: test.NewTestData(userKey, hostKey)}
	td.cfg.KeyFile = td.srvData.UserKeyFile

	var err error
	if td.tempDir, err = ioutil.TempDir("", "local_test."); err != nil {
		td.srvData.Close()
		panic(err)
	}

	td.cfg.ResDir = filepath.Join(td.tempDir, "results")
	if err := os.Mkdir(td.cfg.ResDir, 0755); err != nil {
		panic(err)
	}
	td.cfg.Logger = logging.NewSimple(&td.logbuf, log.LstdFlags, true)
	td.cfg.Target = td.srvData.Srv.Addr().String()

	// Avoid checking test dependencies, which causes an extra local_test_runner call.
	td.cfg.checkTestDeps = checkTestDepsNever

	// Run actual commands when performing file copies.
	td.hostDir = filepath.Join(td.tempDir, "host")
	if err := os.Mkdir(td.hostDir, 0755); err != nil {
		// TODO(derat): Pass *gotesting.T to this function and call t.Fail here and above instead of panicking.
		panic(err)
	}
	td.cfg.hstCopyBasePath = td.hostDir
	td.cfg.hstCopyAnnounceCmd = td.srvData.Srv.NextCmd

	return &td
}

func (td *localTestData) close() {
	td.cfg.Close(context.Background())
	td.srvData.Close()
	os.RemoveAll(td.cfg.ResDir)
}

// addCheckDataFakeCmd registers the command that local uses to check where test data is installed.
// TODO(derat): Remove this after 20180901: https://crbug.com/857485
func addCheckDataFakeCmd(srv *test.SSHServer, status int) {
	dir := filepath.Join(localDataBuiltinDir, localBundlePkgPathPrefix)
	srv.FakeCmd(fmt.Sprintf("test -d "+host.QuoteShellArg(dir)), status, []byte{}, []byte{}, nil)
}

// addLocalRunnerFakeCmd registers the command that local uses to run local_test_runner.
// The returned buffer will contain data written to the command's stdin.
func addLocalRunnerFakeCmd(srv *test.SSHServer, status int, stdout, stderr []byte) (stdin *bytes.Buffer) {
	stdin = &bytes.Buffer{}
	srv.FakeCmd(localRunnerPath, status, stdout, stderr, stdin)
	return stdin
}

// checkArgs unmarshals a runner.Args struct from stdin and compares it to exp.
func checkArgs(t *gotesting.T, stdin io.Reader, exp *runner.Args) {
	args := runner.Args{}
	if err := json.NewDecoder(stdin).Decode(&args); err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(args, *exp) {
		t.Errorf("got args %v; want %v", args, *exp)
	}
}

func TestLocalSuccess(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckDataFakeCmd(td.srvData.Srv, 0)

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, ob.Bytes(), nil)

	if status, _ := local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
	checkArgs(t, stdin, &runner.Args{
		BundleGlob: builtinBundleGlob,
		DataDir:    localDataBuiltinDir,
	})
}

func TestLocalExecFailure(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckDataFakeCmd(td.srvData.Srv, 0)

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
	const stderr = "some failure message\n"
	addLocalRunnerFakeCmd(td.srvData.Srv, 1, ob.Bytes(), []byte(stderr))

	if status, _ := local(context.Background(), &td.cfg); status != subcommands.ExitFailure {
		t.Errorf("local() = %v; want %v", status, subcommands.ExitFailure)
	}
	if !strings.Contains(td.logbuf.String(), stderr) {
		t.Errorf("local() logged %q; want substring %q", td.logbuf.String(), stderr)
	}
}

func TestLocalList(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckDataFakeCmd(td.srvData.Srv, 0)

	tests := []testing.Test{
		testing.Test{Name: "pkg.Test", Desc: "This is a test", Attr: []string{"attr1", "attr2"}},
		testing.Test{Name: "pkg.AnotherTest", Desc: "Another test"},
	}
	b, err := json.Marshal(tests)
	if err != nil {
		t.Fatal(err)
	}
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, b, nil)

	td.cfg.Mode = ListTestsMode
	var status subcommands.ExitStatus
	var results []TestResult
	if status, results = local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
	checkArgs(t, stdin, &runner.Args{
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
	td := newLocalTestData()
	defer td.close()

	const (
		pkg     = "chromiumos/tast/local/bundles/test/pkg"
		pattern = "pkg.*"

		file1 = "file1.txt"
		file2 = "file2.txt"
		file3 = "file3.txt"
		file4 = "file4.txt"
	)

	// Make local_test_runner list two tests containing the first three files (with overlap).
	tests := []testing.Test{
		testing.Test{Name: "pkg.Test1", Pkg: pkg, Data: []string{file1, file2}},
		testing.Test{Name: "pkg.Test2", Pkg: pkg, Data: []string{file2, file3}},
	}
	b, err := json.Marshal(tests)
	if err != nil {
		t.Fatal(err)
	}
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, b, nil)

	// Create a fake source checkout and write the data files to it. Just use their names as their contents.
	td.cfg.buildCfg.TestWorkspace = filepath.Join(td.tempDir, "ws")
	if err := testutil.WriteFiles(filepath.Join(td.cfg.buildCfg.TestWorkspace, "src", tests[0].DataDir()),
		map[string]string{file1: file1, file2: file2, file3: file3, file4: file4}); err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the three files needed by them.
	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal(err)
	}
	td.cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.cfg, td.cfg.hst, builtinBundleGlob)
	if err != nil {
		t.Fatal("getDataFilePaths() failed: ", err)
	}
	checkArgs(t, stdin, &runner.Args{
		Mode:       runner.ListTestsMode,
		BundleGlob: builtinBundleGlob,
		Patterns:   []string{pattern},
	})
	expPaths := []string{
		filepath.Join(tests[0].DataDir(), file1),
		filepath.Join(tests[0].DataDir(), file2),
		filepath.Join(tests[1].DataDir(), file3),
	}
	if !reflect.DeepEqual(paths, expPaths) {
		t.Fatalf("getDataFilePaths() = %v; want %v", paths, expPaths)
	}

	// pushDataFiles should copy the three files to the DUT.
	if err = pushDataFiles(context.Background(), &td.cfg, td.cfg.hst, localDataPushDir, paths); err != nil {
		t.Fatal("pushDataFiles() failed: ", err)
	}
	expData := map[string]string{
		filepath.Join(tests[0].DataDir(), file1): file1,
		filepath.Join(tests[0].DataDir(), file2): file2,
		filepath.Join(tests[1].DataDir(), file3): file3,
	}
	if data, err := testutil.ReadFiles(filepath.Join(td.hostDir, localDataPushDir)); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(data, expData) {
		t.Errorf("pushDataFiles() copied %v; want %v", data, expData)
	}
}

// TODO(derat): Add a test that verifies that getInitialSysInfo is called before tests are run.
