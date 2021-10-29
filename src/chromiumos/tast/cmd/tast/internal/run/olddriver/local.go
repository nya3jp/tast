// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package olddriver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/diagnose"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/debugger"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

const (
	localRunnerWaitTimeout = 10 * time.Second // default timeout for waiting for local_test_runner to exit
	heartbeatInterval      = time.Second      // interval for heartbeat messages
)

type remoteFixtureService struct {
	rpcCL *rpc.ExecClient
	cl    bundle.FixtureService_RunFixtureClient
}

// newRemoteFixtureService executes the remote bundle as a gRPC server and
// returns fixture service connecting to it. The caller should call rf.close
// to gracefully stop the server and the client.
func newRemoteFixtureService(ctx context.Context, cfg *config.Config) (rf *remoteFixtureService, retErr error) {
	serverPath := filepath.Join(cfg.RemoteBundleDir(), cfg.PrimaryBundle())

	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}

	rpcCL, err := rpc.DialExec(ctx, serverPath, 0, false, &protocol.HandshakeRequest{})

	if err != nil {
		return nil, fmt.Errorf("rpc.NewClient: %v", err)
	}
	defer func() {
		if retErr != nil {
			rpcCL.Close()
		}
	}()

	cl, err := bundle.NewFixtureServiceClient(rpcCL.Conn()).RunFixture(ctx)
	if err != nil {
		return nil, fmt.Errorf("RunFixture against %v: %v", serverPath, err)
	}
	return &remoteFixtureService{
		rpcCL: rpcCL,
		cl:    cl,
	}, nil
}

func (rf *remoteFixtureService) close() (retErr error) {
	defer func() {
		if err := rf.rpcCL.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("rpcCL.Close(): %v", err)
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

type bundleTests struct {
	bundle string
	tests  []*categorizedTests
}

type categorizedTests struct {
	remoteFixt string
	tests      []*protocol.Entity
}

// localTestsCategorizer categorizes local by the bundle path and the
// depending remote fixture in this order. Results are sorted by
// the bundle name, the depending remote fixture name, and the test name,
// assuming the input is already sorted by the test name.
type localTestsCategorizer func([]*protocol.Entity) ([]*bundleTests, error)

// newLocalTestsCategorizer creates a function which categorizes given local
// tests by the bundle name and the remote fixture name tests depend on.
// It computes by listing all the fixtures in the bundles designated by cfg.
func newLocalTestsCategorizer(ctx context.Context, cfg *config.Config, drv *driver.Driver) (localTestsCategorizer, error) {
	localFixts, err := drv.ListLocalFixtures(ctx)
	if err != nil {
		return nil, err
	}
	// bundle -> fixture -> parent
	localFixtParent := make(map[string]map[string]string)
	for _, f := range localFixts {
		if _, ok := localFixtParent[f.Bundle]; !ok {
			localFixtParent[f.Bundle] = make(map[string]string)
		}
		localFixtParent[f.Bundle][f.Resolved.GetEntity().GetName()] = f.Resolved.GetEntity().GetFixture()
	}

	remoteFixts, err := drv.ListRemoteFixtures(ctx)
	if err != nil {
		return nil, err
	}

	// TODO(crbug/1177189): allow multiple bundles to define remote fixtures.
	remoteFixtMap := make(map[string][]*driver.BundleEntity)
	for _, f := range remoteFixts {
		remoteFixtMap[f.Bundle] = append(remoteFixtMap[f.Bundle], f)
	}
	if len(remoteFixtMap) > 1 {
		return nil, fmt.Errorf("multiple (%v) bundles define remote fixtures; want <= 1", len(remoteFixtMap))
	}

	// Compute real remote fixtures. Remote bundles can import tast/local/*
	// package and remote bundles may accidentally contain local fixtures.
	// It filters out such fixtures.
	// TODO(crbug/1179162): Disallow remote bundles to import local packages
	// and remove the code to filter out local fixtures.
	remoteFixtSet, err := func() (map[string]struct{}, error) {
		lfs := make(map[string]struct{})
		for _, fs := range localFixtParent {
			for f := range fs {
				lfs[f] = struct{}{}
			}
		}
		rfs := make(map[string]struct{})
		for _, fs := range remoteFixtMap {
			for _, f := range fs {
				if _, ok := lfs[f.Resolved.GetEntity().GetName()]; ok {
					continue
				}
				rfs[f.Resolved.GetEntity().GetName()] = struct{}{}
				if f.Resolved.GetEntity().GetFixture() != "" {
					return nil, fmt.Errorf(`nested remote fixtures are not supported; parent of %v is %v, want ""`, f.Resolved.GetEntity().GetName(), f.Resolved.GetEntity().GetFixture())
				}
			}
		}
		return rfs, nil
	}()
	if err != nil {
		return nil, err
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
		if _, ok := remoteFixtSet[fixt]; ok {
			return fixt, nil
		}
		p, ok := localFixtParent[bundle][fixt]
		if !ok {
			return "", fmt.Errorf("fixture %q not found in bundle %v", fixt, bundle)
		}
		return dependingRemoteFixture(bundle, p)
	}

	categorizeLocalTests := func(localTests []*protocol.Entity) ([]*bundleTests, error) {
		// bundle -> depending remote fixture -> tests
		resMap := make(map[string]map[string][]*protocol.Entity)
		for _, t := range localTests {
			b := t.GetLegacyData().GetBundle()
			if resMap[b] == nil {
				resMap[b] = make(map[string][]*protocol.Entity)
			}
			rf, err := dependingRemoteFixture(b, t.GetFixture())
			if err != nil {
				return nil, fmt.Errorf("test %v: %v", t.GetName(), err)
			}
			resMap[b][rf] = append(resMap[b][rf], t)
		}

		var bundles []string
		for b := range resMap {
			bundles = append(bundles, b)
		}
		sort.Strings(bundles)

		res := make([]*bundleTests, len(bundles), len(bundles))
		for i, b := range bundles {
			var fixts []string
			for f := range resMap[b] {
				fixts = append(fixts, f)
			}
			sort.Strings(fixts)

			res[i] = &bundleTests{
				bundle: b,
				tests:  make([]*categorizedTests, len(fixts), len(fixts)),
			}

			for j, f := range fixts {
				res[i].tests[j] = &categorizedTests{f, resMap[b][f]}
			}
		}
		return res, nil
	}
	return categorizeLocalTests, nil
}

// runFixtureAndTests runs fixture methods before and after runTests.
// fixtErr will be non-nil if fixture errors happen.
// It also stores fixture logs to a file under "fixtures" dir in cfg.ResDir.
func runFixtureAndTests(ctx context.Context, cfg *config.Config, drv *driver.Driver, rfcl bundle.FixtureService_RunFixtureClient, remoteFixt string, runTests func(ctx context.Context, fixtErr []string) error) (retErr error) {
	fixtResDir := filepath.Join(cfg.ResDir(), "fixtures", remoteFixt)
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
	switch cfg.DownloadMode() {
	case planner.DownloadBatch:
		dm = bundle.RunFixtureConfig_BATCH
	case planner.DownloadLazy:
		dm = bundle.RunFixtureConfig_LAZY
	default:
		return fmt.Errorf("unknown mode %v", cfg.DownloadMode())
	}

	var pushErrs []string

	if remoteFixt != "" {
		handleResponses := func() (fixtErrs []*bundle.RunFixtureError, retErr error) {
			logger := logging.NewSinkLogger(logging.LevelInfo, true, logging.NewWriterSink(fixtLogFile))
			ctx := logging.AttachLogger(ctx, logger)

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

					logging.Infof(ctx, "[%s] Error at %s:%d: %s", ts, filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
					if v.Error.Stack != "" {
						logging.Infof(ctx, "[%s] Stack trace:\n%s", ts, v.Error.Stack)
					}
				case *bundle.RunFixtureResponse_Log:
					logging.Infof(ctx, "[%s] %s", ts, v.Log)
				case *bundle.RunFixtureResponse_RequestDone:
					return
				}
			}
		}

		var tlwServer string
		if addr, ok := drv.Services().TLWAddr(); ok {
			tlwServer = addr.String()
		}

		// push
		if err := rfcl.Send(&bundle.RunFixtureRequest{
			Control: &bundle.RunFixtureRequest_Push{
				Push: &bundle.RunFixturePushRequest{
					Name: remoteFixt,
					Config: &bundle.RunFixtureConfig{
						TestVars:          cfg.TestVars(),
						DataDir:           cfg.RemoteDataDir(),
						OutDir:            cfg.RemoteOutDir(),
						TempDir:           "", // empty for fixture service to create it
						ConnectionSpec:    drv.ConnectionSpec(),
						KeyFile:           cfg.KeyFile(),
						KeyDir:            cfg.KeyDir(),
						LocalBundleDir:    cfg.LocalBundleDir(),
						CheckSoftwareDeps: false,
						Devservers:        cfg.Devservers(),
						TlwServer:         tlwServer,
						DutName:           cfg.Target(),
						BuildArtifactsUrl: cfg.BuildArtifactsURL(),
						DownloadMode:      dm,
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

	if err := runTests(ctx, pushErrs); err != nil {
		return fmt.Errorf("runTests(): %v", err)
	}
	return nil
}

// RunLocalTests executes tests as described by cfg on hst and returns the
// results. It is only used for RunnerRunTestsMode.
// It can return partial results and an error when error happens mid-tests.
func RunLocalTests(ctx context.Context, cfg *config.Config, state *config.State, dutInfo *protocol.DUTInfo, drv *driver.Driver) (res []*resultsjson.Result, retErr error) {
	ctx, st := timing.Start(ctx, "run_local_tests")
	defer st.End()

	rf, err := newRemoteFixtureService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rf.close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	categorize, err := newLocalTestsCategorizer(ctx, cfg, drv)
	if err != nil {
		return nil, err
	}

	var tests []*protocol.Entity
	for _, t := range state.TestsToRun {
		if t.Resolved.GetHops() > 0 {
			tests = append(tests, t.Resolved.GetEntity())
		}
	}
	bundleRemoteFixtTests, err := categorize(tests)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	var entityResults []*resultsjson.Result
	for _, bt := range bundleRemoteFixtTests {
		logging.Infof(ctx, "Running tests in bundle %v", bt.bundle)

		for _, fixtTests := range bt.tests {
			remoteFixt := fixtTests.remoteFixt
			tests := fixtTests.tests

			names := make([]string, len(tests))
			for i, t := range tests {
				names[i] = t.GetName()
			}

			// TODO(oka): write a unittest testing a connection to DUT is
			// ensured for remote fixture.
			if err := runFixtureAndTests(ctx, cfg, drv, rf.cl, remoteFixt, func(ctx context.Context, setUpErrs []string) error {
				res, err := runLocalTestsForFixture(ctx, names, remoteFixt, setUpErrs, cfg, state, dutInfo, drv)
				entityResults = append(entityResults, res...)
				return err
			}); err != nil {
				return entityResults, err
			}
		}
	}
	elapsed := time.Since(start)
	logging.Infof(ctx, "Ran %v local test(s) in %v", len(entityResults), elapsed.Round(time.Millisecond))

	return entityResults, nil
}

// runLocalTestsForFixture runs given local tests in between remote fixture
// set up and tear down.
// It can return partial results and an error when error happens mid-tests.
func runLocalTestsForFixture(ctx context.Context, names []string, remoteFixt string, setUpErrs []string, cfg *config.Config, state *config.State, dutInfo *protocol.DUTInfo, drv *driver.Driver) ([]*resultsjson.Result, error) {
	// Reconnect if needed because a remote fixture may have disrupted
	// the SSH connection.
	if err := drv.ReconnectIfNeeded(ctx); err != nil {
		return nil, errors.Wrap(err, "failed reconnecting to target")
	}

	beforeRetry := func(ctx context.Context) bool {
		if err := drv.ReconnectIfNeeded(ctx); err != nil {
			logging.Info(ctx, "Failed reconnecting to target: ", err)
			return false
		}
		return true
	}
	runTests := func(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
		return runLocalTestsOnce(ctx, cfg, state, dutInfo, drv, patterns, remoteFixt, setUpErrs)
	}

	results, err := runTestsWithRetry(ctx, cfg, names, runTests, beforeRetry)
	return results, err
}

// startLocalRunner asynchronously starts local_test_runner on hst and passes args to it.
// args.FillDeprecated() is called first to backfill any deprecated fields for old runners.
// The caller is responsible for terminating the returned process.
func startLocalRunner(ctx context.Context, cfg *config.Config, hst *ssh.Conn, args *jsonprotocol.RunnerArgs) (genericexec.Process, error) {
	args.FillDeprecated()

	debugPort := cfg.DebuggerPort(debugger.LocalTestRunner)
	// Ideally we'd like to check that the port is available on the remote machine,
	// but there's no easy way to do that. We would have to run something like lsof
	// and parse the results here.
	cmdName, cmdArgs := debugger.RewriteDebugCommand(debugPort, cfg.LocalRunner())
	cmd := genericexec.CommandSSH(hst, "env", append(append(localTestRunnerEnvVars(cfg), cmdName), cmdArgs...)...)

	proc, err := cmd.Interact(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start local_test_runner: %v", err)
	}
	if debugPort != 0 {
		logging.Infof(ctx, "Waiting for debugger for local_test_runner on port %d", debugPort)
	}

	go json.NewEncoder(proc.Stdin()).Encode(args)

	return proc, nil
}

// runLocalTestsOnce synchronously runs local_test_runner to run local tests
// matching the supplied patterns (rather than cfg.Patterns).
//
// Results from started tests and the names of tests that should have been
// started but weren't (in the order in which they should've been run) are
// returned.
func runLocalTestsOnce(ctx context.Context, cfg *config.Config, state *config.State, dutInfo *protocol.DUTInfo, drv *driver.Driver, patterns []string, startFixtureName string, setUpErrs []string) (
	results []*resultsjson.Result, unstarted []string, err error) {
	ctx, st := timing.Start(ctx, "run_local_tests_once")
	defer st.End()

	// Older local_test_runner does not create the specified output directory.
	// TODO(crbug.com/1000549): Delete this workaround after 20191001.
	// This workaround costs one round-trip time to the DUT.
	if err := drv.SSHConn().CommandContext(ctx, "mkdir", "-p", cfg.LocalOutDir()).Run(); err != nil {
		return nil, nil, err
	}

	localDevservers := append([]string(nil), cfg.Devservers()...)
	if url, ok := drv.Services().EphemeralDevserverURL(); ok {
		localDevservers = append(localDevservers, url)
	}

	var tlwServer string
	if addr, ok := drv.Services().TLWAddr(); ok {
		tlwServer = addr.String()
	}

	args := jsonprotocol.RunnerArgs{
		Mode: jsonprotocol.RunnerRunTestsMode,
		RunTests: &jsonprotocol.RunnerRunTestsArgs{
			BundleArgs: jsonprotocol.BundleRunTestsArgs{
				FeatureArgs:       *jsonprotocol.FeatureArgsFromProto(cfg.Features(dutInfo.GetFeatures())),
				Patterns:          patterns,
				DataDir:           cfg.LocalDataDir(),
				OutDir:            cfg.LocalOutDir(),
				Devservers:        localDevservers,
				TLWServer:         tlwServer,
				DUTName:           cfg.Target(),
				WaitUntilReady:    cfg.WaitUntilReady(),
				HeartbeatInterval: heartbeatInterval,
				BuildArtifactsURL: cfg.BuildArtifactsURL(),
				DownloadMode:      cfg.DownloadMode(),
				StartFixtureName:  startFixtureName,
				SetUpErrors:       setUpErrs,
				CompanionDUTs:     cfg.CompanionDUTs(),
			},
			BundleGlob: cfg.LocalBundleGlob(),
			Devservers: localDevservers,
			DebugPort:  cfg.DebuggerPort(debugger.LocalBundle),
		},
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	proc, err := startLocalRunner(ctx, cfg, drv.SSHConn(), &args)
	if err != nil {
		return nil, nil, err
	}
	defer proc.Wait(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(proc.Stderr())

	crf := func(src, dst string) error {
		return linuxssh.GetAndDeleteFile(ctx, drv.SSHConn(), src, dst, linuxssh.PreserveSymlinks)
	}
	df := func(ctx context.Context, outDir string) string {
		return diagnoseLocalRunError(ctx, drv, outDir)
	}

	results, unstarted, rerr := readTestOutput(ctx, cfg, state, proc.Stdout(), crf, df)

	canceled := false
	if errors.Is(rerr, ErrTerminate) {
		canceled = true
		cancel()
	}

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	wctx, wcancel := context.WithTimeout(ctx, localRunnerWaitTimeout)
	defer wcancel()
	if err := proc.Wait(wctx); err != nil && !canceled {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}

// diagnoseLocalRunError is used to attempt to diagnose the cause of an error encountered
// while running local tests. It returns a string that can be returned by a diagnoseRunErrorFunc.
// Files useful for diagnosis might be saved under outDir.
func diagnoseLocalRunError(ctx context.Context, drv *driver.Driver, outDir string) string {
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(driver.SSHPingTimeout)) {
		return ""
	}
	if err := drv.SSHConn().Ping(ctx, driver.SSHPingTimeout); err == nil {
		return ""
	}
	return "Lost SSH connection: " + diagnose.SSHDrop(ctx, drv, outDir)
}