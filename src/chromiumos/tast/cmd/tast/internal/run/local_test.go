// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
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

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/jsonprotocol"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/shutil"
	"chromiumos/tast/testutil"
)

const (
	mockLocalRunner       = "/mock/local_test_runner"
	mockLocalBundleDir    = "/mock/local_bundles"
	mockLocalDataDir      = "/mock/local_data"
	mockLocalOutDir       = "/mock/local_out"
	mockBuildArtifactsURL = "gs://mock-images/artifacts/"

	mockLocalBundleGlob = mockLocalBundleDir + "/*"

	defaultBootID = "01234567-89ab-cdef-0123-456789abcdef"

	// symlink to these executables created by newLocalTestData
	fakeRemoteFixtureServer = "fake_remote_fixture_server"
)

var mockDevservers = []string{"192.168.0.1:12345", "192.168.0.2:23456"}

type runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int)

func init() {
	// If the binary was executed via a symlink created by
	// fakeRemoteServerData.setUp, behave like a remote fixture server instead
	// of running unit tests.
	if filepath.Base(os.Args[0]) == fakeRemoteFixtureServer {
		os.Exit(runFakeRemoteFixtureServer())
	}
}

const fakeRemoteServerDataPath = "fake_remote_server_data.json"

type fakeRemoteServerData struct {
	// Fixtures maps fixture name to its behavior. If empty, the server
	// contains no fixture.
	Fixtures map[string]*serializableFakeFixture
}

// setUp sets up a fake remote server binary in tempDir and returns the
// executable path. If fd is nil, it sets up without writing fd to a file.
func (fd *fakeRemoteServerData) setUp(tempDir string) (string, error) {
	exec, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(tempDir, fakeRemoteFixtureServer)

	if err := os.Symlink(exec, path); err != nil {
		return "", err
	}
	if fd == nil {
		return path, nil
	}
	b, err := json.Marshal(fd)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(filepath.Join(filepath.Dir(path), fakeRemoteServerDataPath), b, 0644); err != nil {
		return "", err
	}
	return path, nil
}

type serializableFakeFixture struct {
	SetUpLog      string
	SetUpError    string
	TearDownLog   string
	TearDownError string
}

var _ testing.FixtureImpl = (*serializableFakeFixture)(nil)

func (f *serializableFakeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if f.SetUpLog != "" {
		s.Log(f.SetUpLog)
	}
	if f.SetUpError != "" {
		s.Error(f.SetUpError)
	}
	return nil
}

func (f *serializableFakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.TearDownLog != "" {
		s.Log(f.TearDownLog)
	}
	if f.TearDownError != "" {
		s.Error(f.TearDownError)
	}
}

func (*serializableFakeFixture) Reset(ctx context.Context) error                        { return nil }
func (*serializableFakeFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*serializableFakeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func runFakeRemoteFixtureServer() int {
	if os.Args[1] != "-rpc" {
		log.Fatalf("os.Args[1] = %v, want -rpc", os.Args[1])
	}

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	func() {
		b, err := ioutil.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), fakeRemoteServerDataPath))
		if err != nil {
			return // no config file
		}
		var data fakeRemoteServerData
		if err := json.Unmarshal(b, &data); err != nil {
			log.Fatalf("Remote server: %v", err)
		}
		for name, fixt := range data.Fixtures {
			testing.AddFixture(&testing.Fixture{
				Name: name,
				Impl: fixt,
			})
		}
	}()

	if err := bundle.RunRPCServer(os.Stdin, os.Stdout, nil); err != nil {
		log.Fatalf("Remote server: %v", err)
	}
	return 0
}

// localTestData holds data shared between tests that exercise the local
// function.
type localTestData struct {
	srvData *sshtest.TestData
	logbuf  bytes.Buffer
	cfg     config.Config
	state   config.State
	tempDir string

	hostDir     string // directory simulating root dir on DUT for file copies
	nextCopyCmd string // next "exec" command expected for file copies

	bootID     string // simulated content of boot_id file
	unifiedLog string // simulated content of unified system log for the initial boot (defaultBootID)
	ramOops    string // simulated content of console-ramoops file

	expRunCmd string        // expected "exec" command for starting local_test_runner
	runFunc   runFunc       // called for expRunCmd executions
	runDelay  time.Duration // local_test_runner delay before exiting for run tests mode
}

type localTestDataConfig struct {
	remoteFixtures       []*testing.EntityInfo
	fakeRemoteServerData *fakeRemoteServerData
}

type localTestDataOption func(*localTestDataConfig)

func withFakeRemoteRunnerData(remoteFixtures []*testing.EntityInfo) localTestDataOption {
	return func(cfg *localTestDataConfig) {
		cfg.remoteFixtures = remoteFixtures
	}
}

func withFakeRemoteServerData(data *fakeRemoteServerData) localTestDataOption {
	return func(cfg *localTestDataConfig) {
		cfg.fakeRemoteServerData = data
	}
}

// newLocalTestData performs setup for tests that exercise the local function.
// It calls t.Fatal on error.
// options can be used to apply non-default settings.
func newLocalTestData(t *gotesting.T, opts ...localTestDataOption) *localTestData {
	cfg := &localTestDataConfig{} // default config
	for _, opt := range opts {
		opt(cfg)
	}

	td := localTestData{expRunCmd: "exec env " + mockLocalRunner, bootID: defaultBootID}
	td.srvData = sshtest.NewTestData(td.handleExec)
	td.cfg.KeyFile = td.srvData.UserKeyFile

	toClose := &td
	defer func() { toClose.close() }()

	td.tempDir = testutil.TempDir(t)
	td.cfg.ResDir = filepath.Join(td.tempDir, "results")
	if err := os.Mkdir(td.cfg.ResDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg.Logger = logging.NewSimple(&td.logbuf, true, true)
	td.cfg.Target = td.srvData.Srv.Addr().String()
	td.cfg.LocalRunner = mockLocalRunner
	td.cfg.LocalBundleDir = mockLocalBundleDir
	td.cfg.LocalDataDir = mockLocalDataDir
	td.cfg.LocalOutDir = mockLocalOutDir
	td.cfg.Devservers = mockDevservers
	td.cfg.BuildArtifactsURL = mockBuildArtifactsURL
	td.cfg.DownloadMode = planner.DownloadLazy
	td.cfg.TotalShards = 1
	td.cfg.ShardIndex = 0

	// Set up remote runner.
	b, err := json.Marshal(runner.ListFixturesResult{
		Fixtures: map[string][]*testing.EntityInfo{
			"cros": cfg.remoteFixtures,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	td.cfg.RemoteRunner, err = (&fakeRemoteRunnerData{string(b), "", 0}).setUp(td.tempDir)
	if err != nil {
		t.Fatal(err)
	}
	// Set up remote bundle server.
	td.cfg.RemoteFixtureServer, err = cfg.fakeRemoteServerData.setUp(td.tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Avoid checking test dependencies, which causes an extra local_test_runner call.
	td.cfg.CheckTestDeps = false

	// Ensure that already-set environment variables don't affect unit tests.
	td.cfg.Proxy = config.ProxyNone

	// Run actual commands when performing file copies.
	td.hostDir = filepath.Join(td.tempDir, "host")
	if err := os.Mkdir(td.hostDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg.HstCopyBasePath = td.hostDir

	td.state.LocalDevservers = td.cfg.Devservers

	toClose = nil
	return &td
}

func (td *localTestData) close() {
	if td == nil {
		return
	}
	td.state.Close(context.Background())
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
		if args.Mode == runner.RunTestsMode {
			time.Sleep(td.runDelay)
		}
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
	case "exec mkdir -p " + td.cfg.LocalOutDir:
		req.Start(true)
		req.End(0)
	default:
		req.Start(true)
		req.End(req.RunRealCmd())
	}
}

// checkArgs compares two runner.Args.
func checkArgs(t *gotesting.T, args, exp *runner.Args) {
	t.Helper()
	if diff := cmp.Diff(args, exp, cmp.AllowUnexported(runner.Args{})); diff != "" {
		t.Errorf("Args mismatch (-got +want):\n%v", diff)
	}
}

// errorCounts returns a map from test names in rs to the number of errors reported by each.
// This is useful for tests that just want to quickly check the results of a test run.
// Detailed tests for result generation are in results_test.go.
func errorCounts(rs []*jsonprotocol.EntityResult) map[string]int {
	testErrs := make(map[string]int)
	for _, r := range rs {
		testErrs[r.Name] = len(r.Errors)
	}
	return testErrs
}

func TestLocalSuccess(t *gotesting.T) {
	t.Parallel()

	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
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
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		}
		return 0
	}

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // avoid test being blocked indefinitely
	defer cancel()

	if _, err := runLocalTests(ctx, &td.cfg, &td.state, cc); err != nil {
		t.Errorf("runLocalTest failed: %v", err)
	}
}

func TestLocalProxy(t *gotesting.T) {
	t.Parallel()

	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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
	td.cfg.Proxy = config.ProxyEnv

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

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc); err != nil {
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
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{testName}})
			mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}, OutDir: filepath.Join(td.cfg.LocalOutDir, outName)})
			mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: td.cfg.LocalOutDir})
		case runner.ListFixturesMode:
			json.NewEncoder(stdout).Encode(&runner.ListFixturesResult{})
		}
		return 0
	}

	if err := testutil.WriteFiles(filepath.Join(td.hostDir, td.cfg.LocalOutDir), map[string]string{
		filepath.Join(outName, outFile): outData,
	}); err != nil {
		t.Fatal(err)
	}

	td.cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{
		Name: testName,
	}}}

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc); err != nil {
		t.Fatalf("runLocalTests failed: %v", err)
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

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc); err == nil {
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
	td.cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{Name: "pkg.Foo"}}}
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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
	td.cfg.LocalRunnerWaitTimeout = time.Millisecond

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	if _, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc); err == nil {
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
	td.cfg.BuildWorkspace = filepath.Join(td.tempDir, "ws")
	srcFiles := map[string]string{
		file1:        file1,
		file2:        file2,
		file3:        file3,
		file4:        file4,
		extLinkFile1: extLinkFile1,
		extFile2:     extFile2,
	}
	if err := testutil.WriteFiles(filepath.Join(td.cfg.BuildWorkspace, "src", testing.RelativeDataDir(tests[0].Pkg)), srcFiles); err != nil {
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

	// Connect to the target.
	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	hst, err := cc.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// getDataFilePaths should list the tests and return the files needed by them.
	td.cfg.BuildBundle = bundleName
	td.cfg.Patterns = []string{pattern}
	paths, err := getDataFilePaths(context.Background(), &td.cfg, &td.state, hst)
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
	if err = pushDataFiles(context.Background(), &td.cfg, hst,
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

// TestLocalMaxFailures makes sure that runLocalTests does not run any tests after maximum failures allowed has been reach.
func TestLocalMaxFailures(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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
	td.cfg.TestsToRun = []*jsonprotocol.EntityResult{{EntityInfo: testing.EntityInfo{Name: "pkg.Test"}}}
	td.cfg.MaxTestFailures = 1
	td.state.FailuresCount = 0

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	results, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc)
	if err == nil {
		t.Errorf("runLocalTests() passed unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("runLocalTests return %v results; want 1", len(results))
	}
}

func TestFixturesDependency(t *gotesting.T) {
	td := newLocalTestData(t, withFakeRemoteRunnerData([]*testing.EntityInfo{
		{Name: "remoteFixt"},
		{Name: "failFixt"},
		{Name: "tearDownFailFixt"},
	}), withFakeRemoteServerData(&fakeRemoteServerData{
		Fixtures: map[string]*serializableFakeFixture{
			"remoteFixt":       {SetUpLog: "Hello", TearDownLog: "Bye"},
			"failFixt":         {SetUpError: "Whoa"},
			"tearDownFailFixt": {TearDownError: "Oops"},
			// Local fixtures can be accidentally included (crbug/1179162).
			"fixt1B": {},
		},
	}))
	defer td.close()

	var gotRunArgs []*runner.RunTestsArgs

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
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
	td.cfg.TestsToRun = []*jsonprotocol.EntityResult{
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

	cc := target.NewConnCache(&td.cfg)
	defer cc.Close(context.Background())

	_, err := runLocalTests(context.Background(), &td.cfg, &td.state, cc)
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
		w.BundleGlob = mockLocalBundleGlob
		w.Devservers = mockDevservers
		w.BuildArtifactsURLDeprecated = mockBuildArtifactsURL

		w.BundleArgs.DataDir = mockLocalDataDir
		w.BundleArgs.OutDir = mockLocalOutDir
		w.BundleArgs.Devservers = mockDevservers
		w.BundleArgs.DUTName = td.cfg.Target
		w.BundleArgs.BuildArtifactsURL = mockBuildArtifactsURL
		w.BundleArgs.DownloadMode = planner.DownloadLazy
		w.BundleArgs.HeartbeatInterval = heartbeatInterval
	}

	if diff := cmp.Diff(gotRunArgs, want); diff != "" {
		t.Errorf("Args mismatch (-got +want):\n%v", diff)
	}
}
