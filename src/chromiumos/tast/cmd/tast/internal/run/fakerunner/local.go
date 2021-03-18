// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakerunner

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
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

const (
	// MockLocalRunner is a mock local_test_runner path set by default to LocalTestData.Cfg.
	MockLocalRunner = "/mock/local_test_runner"
	// MockLocalBundleDir is a mock local test bundle dir set by default to LocalTestData.Cfg.
	MockLocalBundleDir = "/mock/local_bundles"
	// MockLocalDataDir is a mock local test data dir set by default to LocalTestData.Cfg.
	MockLocalDataDir = "/mock/local_data"
	// MockLocalOutDir is a mock output dir for local tests set by default to LocalTestData.Cfg.
	MockLocalOutDir = "/mock/local_out"
	// MockBuildArtifactsURL is a mock build artifact URL set by default to LocalTestData.Cfg.
	MockBuildArtifactsURL = "gs://mock-images/artifacts/"

	// MockLocalBundleGlob is a glob matching files under MockLocalBundleDir.
	MockLocalBundleGlob = MockLocalBundleDir + "/*"

	// DefaultBootID is a boot ID used by LocalTestData by default.
	DefaultBootID = "01234567-89ab-cdef-0123-456789abcdef"

	// symlink to these executables created by NewLocalTestData
	fakeRemoteFixtureServer = "fake_remote_fixture_server"
)

// MockDevservers is mock devserver addresses set by default to LocalTestData.Cfg.
var MockDevservers = []string{"192.168.0.1:12345", "192.168.0.2:23456"}

// RunFunc is the type of LocalTestData.RunFunc.
type RunFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int)

func init() {
	// If the binary was executed via a symlink created by
	// FakeRemoteServerData.setUp, behave like a remote fixture server instead
	// of running unit tests.
	if filepath.Base(os.Args[0]) == fakeRemoteFixtureServer {
		os.Exit(runFakeRemoteFixtureServer())
	}
}

const fakeRemoteServerDataPath = "fake_remote_server_data.json"

// FakeRemoteServerData defines remote fixtures to be used in a fake test
// runner.
type FakeRemoteServerData struct {
	// Fixtures maps fixture name to its behavior. If empty, the server
	// contains no fixture.
	Fixtures map[string]*SerializableFakeFixture
}

// setUp sets up a fake remote server binary in tempDir and returns the
// executable path. If fd is nil, it sets up without writing fd to a file.
func (fd *FakeRemoteServerData) setUp(tempDir string) (string, error) {
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

// SerializableFakeFixture is a fake fixture implementation whose behavior can
// be customized with a serializable struct.
type SerializableFakeFixture struct {
	SetUpLog      string
	SetUpError    string
	TearDownLog   string
	TearDownError string
}

var _ testing.FixtureImpl = (*SerializableFakeFixture)(nil)

// SetUp implements fixture setup.
func (f *SerializableFakeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if f.SetUpLog != "" {
		s.Log(f.SetUpLog)
	}
	if f.SetUpError != "" {
		s.Error(f.SetUpError)
	}
	return nil
}

// TearDown implements fixture teardown.
func (f *SerializableFakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.TearDownLog != "" {
		s.Log(f.TearDownLog)
	}
	if f.TearDownError != "" {
		s.Error(f.TearDownError)
	}
}

// Reset implements fixture reset (does nothing).
func (*SerializableFakeFixture) Reset(ctx context.Context) error { return nil }

// PreTest implements fixture pre-test hook (does nothing).
func (*SerializableFakeFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

// PostTest implements fixture post-test hook (does nothing).
func (*SerializableFakeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

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
		var data FakeRemoteServerData
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

// LocalTestData holds data shared between tests that exercise the local
// function.
type LocalTestData struct {
	SrvData *sshtest.TestData
	LogBuf  bytes.Buffer
	Cfg     config.Config
	State   config.State
	TempDir string

	HostDir     string // directory simulating root dir on DUT for file copies
	NextCopyCmd string // next "exec" command expected for file copies

	BootID     string // simulated content of boot_id file
	UnifiedLog string // simulated content of unified system log for the initial boot (DefaultBootID)
	Ramoops    string // simulated content of console-ramoops file

	ExpRunCmd string        // expected "exec" command for starting local_test_runner
	RunFunc   RunFunc       // called for ExpRunCmd executions
	RunDelay  time.Duration // local_test_runner delay before exiting for run tests mode
}

type localTestDataConfig struct {
	remoteFixtures       []*jsonprotocol.EntityInfo
	fakeRemoteServerData *FakeRemoteServerData
	CompanionDUTRoles    []string
}

// LocalTestDataOption is an option that can be passed to NewLocalTestData to
// customize its behavior.
type LocalTestDataOption func(*localTestDataConfig)

// WithFakeRemoteRunnerData is an option that can be passed to NewLocalTestData
// to specify remote fixture entity data.
func WithFakeRemoteRunnerData(remoteFixtures []*jsonprotocol.EntityInfo) LocalTestDataOption {
	return func(cfg *localTestDataConfig) {
		cfg.remoteFixtures = remoteFixtures
	}
}

// WithFakeRemoteServerData is an option that can be passed to NewLocalTestData
// to give remote fixture implementations.
func WithFakeRemoteServerData(data *FakeRemoteServerData) LocalTestDataOption {
	return func(cfg *localTestDataConfig) {
		cfg.fakeRemoteServerData = data
	}
}

// WithCompanionDUTRoles is an option that can be passed to NewLocalTestData
// to give roles to companion DUTs.
func WithCompanionDUTRoles(roles []string) LocalTestDataOption {
	return func(cfg *localTestDataConfig) {
		cfg.CompanionDUTRoles = roles
	}
}

// NewLocalTestData performs setup for tests that exercise the local function.
// It calls t.Fatal on error.
// options can be used to apply non-default settings.
func NewLocalTestData(t *gotesting.T, opts ...LocalTestDataOption) *LocalTestData {
	cfg := &localTestDataConfig{} // default config
	for _, opt := range opts {
		opt(cfg)
	}

	td := LocalTestData{ExpRunCmd: "exec env " + MockLocalRunner, BootID: DefaultBootID}

	handlers := []sshtest.ExecHandler{td.handleExec}
	for i := 0; i < len(cfg.CompanionDUTRoles); i++ {
		handlers = append(handlers, td.handleExec)
	}
	td.SrvData = sshtest.NewTestData(handlers...)
	td.Cfg.KeyFile = td.SrvData.UserKeyFile

	toClose := &td
	defer func() { toClose.Close() }()

	td.TempDir = testutil.TempDir(t)
	td.Cfg.ResDir = filepath.Join(td.TempDir, "results")
	if err := os.Mkdir(td.Cfg.ResDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.Cfg.Logger = logging.NewSimple(&td.LogBuf, true, true)
	td.Cfg.Target = td.SrvData.Srvs[0].Addr().String()
	if len(cfg.CompanionDUTRoles) > 0 {
		srvIndex := 1
		td.Cfg.CompanionDUTs = make(map[string]string)
		for _, role := range cfg.CompanionDUTRoles {
			td.Cfg.CompanionDUTs[role] = td.SrvData.Srvs[srvIndex].Addr().String()
			srvIndex++
		}
	}

	td.Cfg.LocalRunner = MockLocalRunner
	td.Cfg.LocalBundleDir = MockLocalBundleDir
	td.Cfg.LocalDataDir = MockLocalDataDir
	td.Cfg.LocalOutDir = MockLocalOutDir
	td.Cfg.Devservers = MockDevservers
	td.Cfg.BuildArtifactsURL = MockBuildArtifactsURL
	td.Cfg.DownloadMode = planner.DownloadLazy
	td.Cfg.TotalShards = 1
	td.Cfg.ShardIndex = 0

	// Set up remote runner.
	b, err := json.Marshal(runner.ListFixturesResult{
		Fixtures: map[string][]*jsonprotocol.EntityInfo{
			"cros": cfg.remoteFixtures,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	td.Cfg.RemoteRunner, err = (&fakeRemoteRunnerData{string(b), "", 0}).setUp(td.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	// Set up remote bundle server.
	td.Cfg.RemoteFixtureServer, err = cfg.fakeRemoteServerData.setUp(td.TempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Avoid checking test dependencies, which causes an extra local_test_runner call.
	td.Cfg.CheckTestDeps = false

	// Ensure that already-set environment variables don't affect unit tests.
	td.Cfg.Proxy = config.ProxyNone

	// Run actual commands when performing file copies.
	td.HostDir = filepath.Join(td.TempDir, "host")
	if err := os.Mkdir(td.HostDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.Cfg.HstCopyBasePath = td.HostDir

	toClose = nil
	return &td
}

// Close releases resources associated to LocalTestData.
func (td *LocalTestData) Close() {
	if td == nil {
		return
	}
	td.State.Close(context.Background())
	td.SrvData.Close()
	if td.TempDir != "" {
		os.RemoveAll(td.TempDir)
	}
}

// handleExec handles SSH "exec" requests sent to td.SrvData.Srv.
// Canned results are returned for local_test_runner, while file-copying-related commands are actually executed.
func (td *LocalTestData) handleExec(req *sshtest.ExecReq) {
	switch req.Cmd {
	case "exec cat /proc/sys/kernel/random/boot_id":
		req.Start(true)
		fmt.Fprintln(req, td.BootID)
		req.End(0)
	case td.ExpRunCmd:
		req.Start(true)
		var args runner.Args
		var status int
		if err := json.NewDecoder(req).Decode(&args); err != nil {
			status = command.WriteError(req.Stderr(), err)
		} else {
			status = td.RunFunc(&args, req, req.Stderr())
		}
		req.CloseOutput()
		if args.Mode == runner.RunTestsMode {
			time.Sleep(td.RunDelay)
		}
		req.End(status)
	case "exec sync":
		req.Start(true)
		req.End(0)
	case "exec croslog --quiet --boot=" + strings.Replace(DefaultBootID, "-", "", -1) + " --lines=1000":
		req.Start(true)
		io.WriteString(req, td.UnifiedLog)
		req.End(0)
	case "exec cat /sys/fs/pstore/console-ramoops", "exec cat /sys/fs/pstore/console-ramoops-0":
		req.Start(true)
		io.WriteString(req, td.Ramoops)
		req.End(0)
	case "exec mkdir -p " + td.Cfg.LocalOutDir:
		req.Start(true)
		req.End(0)
	default:
		req.Start(true)
		req.End(req.RunRealCmd())
	}
}
