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
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

const (
	sshConnectTimeout = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshPingTimeout    = 5 * time.Second  // timeout for checking if SSH connection to DUT is open
	sshRetryInterval  = 5 * time.Second  // minimum time to wait between SSH connection attempts

	defaultLocalRunnerWaitTimeout = 10 * time.Second // default timeout for waiting for local_test_runner to exit
	heartbeatInterval             = time.Second      // interval for heartbeat messages
)

// connectToTarget establishes an SSH connection to the target specified in cfg.
// The connection will be cached in cfg and should not be closed by the caller.
// If a connection is already established, it will be returned.
func connectToTarget(ctx context.Context, cfg *Config) (*ssh.Conn, error) {
	// If we already have a connection, reuse it if it's still open.
	if cfg.hst != nil {
		if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
			return cfg.hst, nil
		}
		cfg.hst = nil
	}

	ctx, st := timing.Start(ctx, "connect")
	defer st.End()
	cfg.Logger.Status("Connecting to target")
	cfg.Logger.Logf("Connecting to %s", cfg.Target)

	o := ssh.Options{
		ConnectTimeout:       sshConnectTimeout,
		ConnectRetries:       cfg.sshRetries,
		ConnectRetryInterval: sshRetryInterval,
		KeyFile:              cfg.KeyFile,
		KeyDir:               cfg.KeyDir,
		WarnFunc:             func(s string) { cfg.Logger.Log(s) },
	}
	if err := ssh.ParseTarget(cfg.Target, &o); err != nil {
		return nil, err
	}

	var err error
	if cfg.hst, err = ssh.New(ctx, &o); err != nil {
		return nil, err
	}

	if cfg.initBootID == "" {
		if cfg.initBootID, err = readBootID(ctx, cfg.hst); err != nil {
			return nil, err
		}
	}

	return cfg.hst, nil
}

type remoteFixtureService struct {
	cmd *exec.Cmd // remote fixture gRPC server
	cl  bundle.FixtureService_RunFixtureClient
}

// newRemoteFixtureService executes the remote bundle as a gRPC server and
// returns fixture service connecting to it. The caller should call rf.close
// to gracefully stop the server and the client.
func newRemoteFixtureService(ctx context.Context, cfg *Config) (rf *remoteFixtureService, retErr error) {
	if _, err := os.Stat(cfg.remoteFixtureServer); os.IsNotExist(err) {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}

	cmd := exec.CommandContext(ctx, cfg.remoteFixtureServer, "-fixtureservice")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	cmd.Stderr = os.Stderr // ease debug
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	defer func() {
		if retErr != nil {
			cmd.Process.Kill() // no error handling; already failed
			cmd.Wait()
		}
	}()

	conn, err := rpc.NewPipeClientConn(ctx, stdout, stdin, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:    10 * time.Second, // ping interval
		Timeout: 1 * time.Minute,
	}))
	if err != nil {
		return nil, fmt.Errorf("NewPipeClientConn: %v", err)
	}
	cl, err := bundle.NewFixtureServiceClient(conn).RunFixture(ctx)
	if err != nil {
		return nil, fmt.Errorf("RunFixture: %v", err)
	}
	return &remoteFixtureService{
		cmd: cmd,
		cl:  cl,
	}, nil
}

func (rf *remoteFixtureService) close() (retErr error) {
	defer func() {
		if err := rf.cmd.Process.Kill(); err != nil && retErr == nil {
			retErr = fmt.Errorf("rf.close: %v", err)
		}
		if err := rf.cmd.Wait(); err != nil && retErr == nil {
			retErr = fmt.Errorf("rf.close: %v", err)
		}
	}()
	if err := rf.cl.CloseSend(); err != nil {
		return fmt.Errorf("rf.close: %v", err)
	}
	if r, err := rf.cl.Recv(); err != io.EOF {
		return fmt.Errorf("rf.close: cl.Recv() = %v, %v, want EOF", r, err)
	}
	return nil
}

// localTestsCategorizer categorizes local by the bundle path and the
// depending remote fixture in this order.
type localTestsCategorizer func([]*testing.EntityInfo) (map[string]map[string][]*testing.EntityInfo, error)

// newLocalTestsCategorizer creates a function which categorizes given local
// tests by the bundle name and the remote fixture name tests depend on.
// It computes by listing all the fixtures in the bundles designated by cfg.
func newLocalTestsCategorizer(ctx context.Context, cfg *Config) (localTestsCategorizer, error) {
	var remoteFixts runner.ListFixturesResult
	if err := runTestRunnerCommand(remoteRunnerCommand(ctx, cfg), &runner.Args{
		Mode: runner.ListFixturesMode,
		ListFixtures: &runner.ListFixturesArgs{
			BundleGlob: cfg.remoteBundleGlob(),
		},
	},
		&remoteFixts,
	); err != nil {
		return nil, fmt.Errorf("list remote fixtures: %v", err)
	}

	var localFixts runner.ListFixturesResult
	if err := runTestRunnerCommand(localRunnerCommand(ctx, cfg, cfg.hst), &runner.Args{
		Mode: runner.ListFixturesMode,
		ListFixtures: &runner.ListFixturesArgs{
			BundleGlob: cfg.localBundleGlob(),
		},
	},
		&localFixts,
	); err != nil {
		return nil, fmt.Errorf("listing local fixtures: %v", err)
	}

	if got := len(remoteFixts.Fixtures); got > 1 {
		return nil, fmt.Errorf("BUG: got %v remote bundles, want 1", got)
	}

	remoteFixtSet := make(map[string]bool)
	for _, fs := range remoteFixts.Fixtures {
		for _, f := range fs {
			remoteFixtSet[f.Name] = true
			if f.Fixture != "" {
				return nil, fmt.Errorf(`nested remote fixtures are not supported; parent of %v is %v, want ""`, f.Name, f.Fixture)
			}
		}
	}
	// bundle -> fixture -> parent
	localFixtParent := make(map[string]map[string]string)
	for bundlePath, fs := range localFixts.Fixtures {
		bundle := filepath.Base(bundlePath)
		localFixtParent[bundle] = make(map[string]string)
		for _, f := range fs {
			localFixtParent[bundle][f.Name] = f.Fixture
		}
	}

	// Check there's no duplicated fixtures between remote and local bundles.
	for bundle, fixts := range localFixtParent {
		for fixt := range fixts {
			if _, ok := remoteFixtSet[fixt]; ok {
				return nil, fmt.Errorf("both local bundle %v and the remote bundle has fixture %v", bundle, fixt)
			}
		}
	}

	var dependingRemoteFixture func(string, string) (string, error)
	dependingRemoteFixture = func(bundle, fixt string) (remoteFixt string, err error) {
		if fixt == "" {
			return "", nil
		}
		if remoteFixtSet[fixt] {
			return fixt, nil
		}
		p, ok := localFixtParent[bundle][fixt]
		if !ok {
			return "", fmt.Errorf("fixture %q not found in bundle %v", fixt, bundle)
		}
		return dependingRemoteFixture(bundle, p)
	}

	categorizeLocalTests := func(localTests []*testing.EntityInfo) (map[string]map[string][]*testing.EntityInfo, error) {
		// bundle -> depending remote fixture -> tests
		res := make(map[string]map[string][]*testing.EntityInfo)
		for _, t := range localTests {
			if res[t.Bundle] == nil {
				res[t.Bundle] = make(map[string][]*testing.EntityInfo)
			}
			rf, err := dependingRemoteFixture(t.Bundle, t.Fixture)
			if err != nil {
				return nil, fmt.Errorf("test %v: %v", t.Name, err)
			}
			res[t.Bundle][rf] = append(res[t.Bundle][rf], t)
		}
		return res, nil
	}
	return categorizeLocalTests, nil
}

// runFixtureAndTests runs fixture methods before and after runTests.
// fixtErr will be non-nil if fixture errors happen.
// It also stores fixture logs to a file under "fixtures" dir in cfg.ResDir.
func runFixtureAndTests(ctx context.Context, cfg *Config, rfcl bundle.FixtureService_RunFixtureClient, remoteFixt string, runTests func(fixtErr []string) error) (retErr error) {
	fixtResDir := filepath.Join(cfg.ResDir, "fixtures", remoteFixt)
	// TODO(oka) rename testLogFilename to entityLogFilename
	fixtLogPath := filepath.Join(fixtResDir, testLogFilename)

	if err := os.MkdirAll(filepath.Dir(fixtLogPath), 0755); err != nil {
		return err
	}
	fixtLogFile, err := os.Create(fixtLogPath)
	if err != nil {
		return fmt.Errorf("fixtLogFile: %v", err)
	}
	defer func() {
		if err := fixtLogFile.Close(); err != nil && retErr != nil {
			retErr = fmt.Errorf("fixtLogFile: %v", err)
		}
	}()

	var dm bundle.RunFixtureConfig_PlannerDownloadMode
	switch cfg.downloadMode {
	case planner.DownloadBatch:
		dm = bundle.RunFixtureConfig_BATCH
	case planner.DownloadLazy:
		dm = bundle.RunFixtureConfig_LAZY
	default:
		return fmt.Errorf("unknown mode %v", cfg.downloadMode)
	}

	var pushErrs []string

	if remoteFixt != "" {
		handleResponses := func() (fixtErrs []*bundle.RunFixtureError, retErr error) {
			if err := cfg.Logger.AddWriter(fixtLogFile, log.LstdFlags); err != nil {
				return nil, fmt.Errorf("handle fixture log: %v", err)
			}
			defer func() {
				if err := cfg.Logger.RemoveWriter(fixtLogFile); err != nil && retErr == nil {
					retErr = fmt.Errorf("handle fixture log: %v", err)
				}
			}()

			for {
				msg, err := rfcl.Recv()
				if err != nil {
					return nil, fmt.Errorf("rfcl.Recv(): %v", err)
				}

				timestamp, err := ptypes.Timestamp(msg.Timestamp)
				if err != nil {
					return nil, fmt.Errorf("invalid timestamp: %v", err)
				}
				ts := timestamp.Format(testOutputTimeFmt)

				switch v := msg.Control.(type) {
				case *bundle.RunFixtureResponse_Error:
					fixtErrs = append(fixtErrs, v.Error)

					cfg.Logger.Logf("[%s] Error at %s:%d: %s", ts, filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
					if v.Error.Stack != "" {
						cfg.Logger.Logf("[%s] Stack trace:\n%s", ts, v.Error.Stack)
					}
				case *bundle.RunFixtureResponse_Log:
					cfg.Logger.Logf("[%s] %s", ts, v.Log)
				case *bundle.RunFixtureResponse_RequestDone:
					return
				}
			}
		}

		if cfg.softwareFeatures == nil {
			cfg.softwareFeatures = &dep.SoftwareFeatures{}
		}
		// push
		if err := rfcl.Send(&bundle.RunFixtureRequest{
			Control: &bundle.RunFixtureRequest_Push{
				Push: &bundle.RunFixturePushRequest{
					Name: remoteFixt,
					Config: &bundle.RunFixtureConfig{
						TestVars:                    cfg.testVars,
						DataDir:                     cfg.remoteDataDir,
						OutDir:                      cfg.remoteOutDir,
						TempDir:                     "", // empty for fixture service to create it
						Target:                      cfg.Target,
						KeyFile:                     cfg.KeyFile,
						KeyDir:                      cfg.KeyDir,
						LocalBundleDir:              cfg.localBundleDir,
						CheckSoftwareDeps:           true,
						AvailableSoftwareFeatures:   cfg.softwareFeatures.Available,
						UnavailableSoftwareFeatures: cfg.softwareFeatures.Unavailable,
						Devservers:                  cfg.devservers,
						TlwServer:                   cfg.tlwServerForDUT,
						DutName:                     cfg.Target,
						BuildArtifactsUrl:           cfg.buildArtifactsURL,
						DownloadMode:                dm,
					},
				},
			},
		}); err != nil {
			return fmt.Errorf("push %v: %v", remoteFixt, err)
		}

		fixtErrs, err := handleResponses()
		if err != nil {
			return fmt.Errorf("push %v: %v", remoteFixt, err)
		}
		for _, e := range fixtErrs {
			pushErrs = append(pushErrs, e.Reason)
		}

		defer func() { // pop after tests run
			if err := rfcl.Send(&bundle.RunFixtureRequest{
				Control: &bundle.RunFixtureRequest_Pop{
					Pop: &bundle.RunFixturePopRequest{},
				},
			}); err != nil && retErr == nil {
				retErr = fmt.Errorf("pop: %v", err)
			}

			// fixtErrs is not used. Fixture errors are reported to the logger
			// and handled there.
			_, err := handleResponses()
			if err != nil && retErr == nil {
				retErr = fmt.Errorf("pop: %v", err)
			}
		}()
	}

	if err := runTests(pushErrs); err != nil {
		return fmt.Errorf("runTests(): %v", err)
	}
	return nil
}

// runLocalTests executes tests as described by cfg on hst and returns the
// results. It is only used for RunTestsMode.
func runLocalTests(ctx context.Context, cfg *Config) ([]*EntityResult, error) {
	cfg.Logger.Status("Running local tests on target")
	ctx, st := timing.Start(ctx, "run_local_tests")
	defer st.End()

	_, err := connectToTarget(ctx, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", cfg.Target)
	}

	rf, err := newRemoteFixtureService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer rf.close()

	categorize, err := newLocalTestsCategorizer(ctx, cfg)
	if err != nil {
		return nil, err
	}

	tests := make([]*testing.EntityInfo, len(cfg.testsToRun), len(cfg.testsToRun))
	for i, t := range cfg.testsToRun {
		tests[i] = &t.EntityInfo
	}
	bundleRemoteFixtTests, err := categorize(tests)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	var entityResults []*EntityResult
	for bundle, remoteFixtTests := range bundleRemoteFixtTests {
		cfg.Logger.Logf("Running tests in bundle %v", bundle)

		for remoteFixt, tests := range remoteFixtTests {
			names := make([]string, len(tests), len(tests))
			for i, t := range tests {
				names[i] = t.Name
			}

			// TODO(oka): write a unittest testing a connection to DUT is
			// ensured for remote fixture.
			if err := runFixtureAndTests(ctx, cfg, rf.cl, remoteFixt, func(setUpErrs []string) error {
				res, err := runLocalTestsSub(ctx, names, remoteFixt, setUpErrs, cfg)
				if err != nil {
					return err
				}
				entityResults = append(entityResults, res...)
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	elapsed := time.Since(start)
	cfg.Logger.Logf("Ran %v local test(s) in %v", len(entityResults), elapsed.Round(time.Millisecond))

	return entityResults, nil
}

// runLocalTestsSub runs given local tests in between remote fixture
// set up and tear down.
func runLocalTestsSub(ctx context.Context, names []string, remoteFixt string, setUpErrs []string, cfg *Config) ([]*EntityResult, error) {
	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s; remoteFixt = %q", cfg.Target, remoteFixt)
	}
	beforeRetry := func(ctx context.Context) bool {
		oldHst := hst
		var connErr error
		if hst, connErr = connectToTarget(ctx, cfg); connErr != nil {
			cfg.Logger.Log("Failed reconnecting to target: ", connErr)
			return false
		}
		// The ephemeral devserver uses the SSH connection to the DUT, so a new devserver needs
		// to be created if a new SSH connection was established.
		if hst != oldHst {
			if cfg.ephemeralDevserver != nil {
				if devErr := startEphemeralDevserver(ctx, hst, cfg); devErr != nil {
					cfg.Logger.Log("Failed restarting ephemeral devserver: ", connErr)
					return false
				}
			}
			if cfg.tlwServerForDUT != "" {
				f, err := hst.ForwardRemoteToLocal("tcp", "127.0.0.1:0", cfg.tlwServer, func(e error) {
					cfg.Logger.Logf("remote forwarder error: %s", e)
				})
				if err != nil {
					cfg.Logger.Log("Failed reconnecting remote port forwarding: ", err)
					return false
				}
				cfg.tlwServerForDUT = f.ListenAddr().String()
			}
		}
		return true
	}
	runTests := func(ctx context.Context, patterns []string) (results []*EntityResult, unstarted []string, err error) {
		return runLocalTestsOnce(ctx, cfg, hst, patterns, remoteFixt, setUpErrs)
	}

	results, err := runTestsWithRetry(ctx, cfg, names, runTests, beforeRetry)
	return results, err
}

type localRunnerHandle struct {
	cmd            *ssh.Cmd
	stdout, stderr io.Reader
}

// Close kills and waits the remote process.
func (h *localRunnerHandle) Close(ctx context.Context) error {
	h.cmd.Abort()
	return h.cmd.Wait(ctx)
}

// startLocalRunner asynchronously starts local_test_runner on hst and passes args to it.
// args.FillDeprecated() is called first to backfill any deprecated fields for old runners.
// The caller is responsible for reading the handle's stdout and closing the handle.
func startLocalRunner(ctx context.Context, cfg *Config, hst *ssh.Conn, args *runner.Args) (*localRunnerHandle, error) {
	args.FillDeprecated()
	argsData, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %v", err)
	}

	cmd := localRunnerCommand(ctx, cfg, hst)
	cmd.cmd.Stdin = bytes.NewBuffer(argsData)
	stdout, err := cmd.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe: %v", err)
	}
	stderr, err := cmd.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stderr pipe: %v", err)
	}

	if err := cmd.cmd.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start local_test_runner: %v", err)
	}
	return &localRunnerHandle{cmd.cmd, stdout, stderr}, nil
}

// runLocalTestsOnce synchronously runs local_test_runner to run local tests
// matching the supplied patterns (rather than cfg.Patterns).
//
// Results from started tests and the names of tests that should have been
// started but weren't (in the order in which they should've been run) are
// returned.
func runLocalTestsOnce(ctx context.Context, cfg *Config, hst *ssh.Conn, patterns []string, startFixtureName string, setUpErrs []string) (
	results []*EntityResult, unstarted []string, err error) {
	ctx, st := timing.Start(ctx, "run_local_tests_once")
	defer st.End()

	// Older local_test_runner does not create the specified output directory.
	// TODO(crbug.com/1000549): Delete this workaround after 20191001.
	// This workaround costs one round-trip time to the DUT.
	if err := hst.Command("mkdir", "-p", cfg.localOutDir).Run(ctx); err != nil {
		return nil, nil, err
	}
	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				FeatureArgs:       *featureArgsFromConfig(cfg),
				Patterns:          patterns,
				DataDir:           cfg.localDataDir,
				OutDir:            cfg.localOutDir,
				Devservers:        cfg.devservers,
				TLWServer:         cfg.tlwServerForDUT,
				DUTName:           cfg.Target,
				WaitUntilReady:    cfg.waitUntilReady,
				HeartbeatInterval: heartbeatInterval,
				BuildArtifactsURL: cfg.buildArtifactsURL,
				DownloadMode:      cfg.downloadMode,
				StartFixtureName:  startFixtureName,
				SetUpErrors:       setUpErrs,
			},
			BundleGlob: cfg.localBundleGlob(),
			Devservers: cfg.devservers,
		},
	}

	handle, err := startLocalRunner(ctx, cfg, hst, &args)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Close(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(handle.stderr)

	crf := func(src, dst string) error {
		return moveFromHost(ctx, cfg, hst, src, dst)
	}
	df := func(ctx context.Context, outDir string) string {
		return diagnoseLocalRunError(ctx, cfg, outDir)
	}
	results, unstarted, rerr := readTestOutput(ctx, cfg, handle.stdout, crf, df)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	timeout := defaultLocalRunnerWaitTimeout
	if cfg.localRunnerWaitTimeout > 0 {
		timeout = cfg.localRunnerWaitTimeout
	}
	wctx, wcancel := context.WithTimeout(ctx, timeout)
	defer wcancel()
	if err := handle.cmd.Wait(wctx); err != nil {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}

// formatBytes formats bytes as a human-friendly string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float32(bytes)/float32(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float32(bytes)/float32(kb))
	}
	return fmt.Sprintf("%d B", bytes)
}

// startEphemeralDevserver starts an ephemeral devserver serving on hst.
// cfg's ephemeralDevserver and devservers fields are updated.
// If ephemeralDevserver is non-nil, it is closed first.
func startEphemeralDevserver(ctx context.Context, hst *ssh.Conn, cfg *Config) error {
	closeEphemeralDevserver(ctx, cfg) // ignore errors; this may rely on a now-dead SSH connection

	lis, err := hst.ListenTCP(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ephemeralDevserverPort})
	if err != nil {
		return fmt.Errorf("failed to reverse-forward a port: %v", err)
	}

	cacheDir := filepath.Join(cfg.tastDir, "devserver", "static")
	es, err := newEphemeralDevserver(lis, cacheDir, cfg.extraAllowedBuckets)
	if err != nil {
		return err
	}

	cfg.ephemeralDevserver = es
	cfg.devservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	return nil
}

// closeEphemeralDevserver closes and resets cfg.ephemeralDevserver if non-nil.
func closeEphemeralDevserver(ctx context.Context, cfg *Config) error {
	var err error
	if cfg.ephemeralDevserver != nil {
		err = cfg.ephemeralDevserver.Close(ctx)
		cfg.ephemeralDevserver = nil
	}
	return err
}

// diagnoseLocalRunError is used to attempt to diagnose the cause of an error encountered
// while running local tests. It returns a string that can be returned by a diagnoseRunErrorFunc.
// Files useful for diagnosis might be saved under outDir.
func diagnoseLocalRunError(ctx context.Context, cfg *Config, outDir string) string {
	if cfg.hst == nil || ctxutil.DeadlineBefore(ctx, time.Now().Add(sshPingTimeout)) {
		return ""
	}
	if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
		return ""
	}
	return "Lost SSH connection: " + diagnoseSSHDrop(ctx, cfg, outDir)
}
