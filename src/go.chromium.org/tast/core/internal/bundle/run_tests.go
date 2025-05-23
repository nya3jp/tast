// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"log/syslog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/command"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/planner"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testcontext"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/timing"
)

// testsToRun returns a sorted list of tests to run for the given patterns.
func testsToRun(scfg *StaticConfig, patterns []string) ([]*testing.TestInstance, error) {
	m, err := testing.NewMatcher(patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", patterns, err.Error())
	}
	var tests []*testing.TestInstance
	for _, t := range scfg.registry.AllTests() {
		if m.Match(t.Name, t.Attr) {
			tests = append(tests, t)
		}
	}
	for _, tp := range tests {
		if tp.Timeout == 0 {
			tp.Timeout = scfg.defaultTestTimeout
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})
	return tests, nil
}

// runTests runs tests per cfg and scfg and writes responses to srv.
//
// If an error is encountered in the test harness (as opposed to in a test), an error is returned.
// Otherwise, nil is returned (test errors will be reported via EntityError control messages).
func runTests(ctx context.Context, srv protocol.TestService_RunTestsServer, cfg *protocol.RunConfig, scfg *StaticConfig, bcfg *protocol.BundleConfig) (retErr error) {
	ctx = testcontext.WithPrivateData(ctx, testcontext.PrivateData{
		WaitUntilReady:        cfg.GetWaitUntilReady(),
		WaitUntilReadyTimeout: cfg.GetWaitUntilReadyTimeout().AsDuration(),
	})

	ew := newEventWriter(srv)

	hbw := newHeartbeatWriter(ew)
	defer hbw.Stop()

	logger := logging.NewFuncLogger(ew.RunLog)
	ctx = logging.AttachLoggerNoPropagation(ctx, logger)

	tests, err := testsToRun(scfg, cfg.GetTests())
	if err != nil {
		return err
	}

	env, err := setUpTestEnvironment(ctx, scfg, cfg, bcfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := env.close(ctx); err != nil && retErr == nil {
			retErr = err
		}
	}()

	connEnv, err := setUpConnection(ctx, scfg, cfg, bcfg)
	if err != nil {
		return err
	}
	defer connEnv.close(ctx)

	pcfg := &planner.Config{
		Dirs:             cfg.GetDirs(),
		Features:         cfg.GetFeatures(),
		Service:          cfg.GetServiceConfig(),
		DataFile:         cfg.GetDataFileConfig(),
		RemoteData:       connEnv.rd,
		TestHook:         scfg.testHook,
		BeforeDownload:   scfg.beforeDownload,
		Fixtures:         scfg.registry.AllFixtures(),
		StartFixtureName: cfg.GetStartFixtureState().GetName(),
		StartFixtureImpl: &stubFixture{setUpErrors: cfg.GetStartFixtureState().GetErrors()},
		MaxSysMsgLogSize: cfg.GetMaxSysMsgLogSize(),
	}

	if err := planner.RunTestsLegacy(ctx, tests, ew, pcfg); err != nil {
		return command.NewStatusErrorf(statusError, "run failed: %v", err)
	}
	return nil
}

// eventWriter wraps MessageWriter to write events to syslog in parallel.
//
// eventWriter is goroutine-safe; it is safe to call its methods concurrently from multiple
// goroutines.
type eventWriter struct {
	srv protocol.TestService_RunTestsServer
	lg  *syslog.Writer
	mu  sync.Mutex // used to synchronize Send calls to srv
}

var _ planner.OutputStream = (*eventWriter)(nil)

func newEventWriter(srv protocol.TestService_RunTestsServer) *eventWriter {
	// Continue even if we fail to connect to syslog.
	lg, _ := syslog.New(syslog.LOG_INFO, "tast")
	return &eventWriter{srv: srv, lg: lg}
}

func (ew *eventWriter) RunLog(level logging.Level, ts time.Time, msg string) {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ew.lg != nil {
		ew.lg.Info(msg)
	}
	ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_RunLog{RunLog: &protocol.RunLogEvent{
		Time:  timestamppb.New(ts),
		Text:  msg,
		Level: protocol.LevelToProto(level),
	}}})
}

func (ew *eventWriter) EntityStart(ei *protocol.Entity, outDir string) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", ei.Name))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityStart{EntityStart: &protocol.EntityStartEvent{
		Time:   timestamppb.Now(),
		Entity: ei,
		OutDir: outDir,
	}}})
}

func (ew *eventWriter) EntityLog(ei *protocol.Entity, level logging.Level, ts time.Time, msg string) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ei.Name, msg))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityLog{EntityLog: &protocol.EntityLogEvent{
		Time:       timestamppb.New(ts),
		EntityName: ei.GetName(),
		Text:       msg,
		Level:      protocol.LevelToProto(level),
	}}})
}

func (ew *eventWriter) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ew.lg != nil {
		loc := e.GetLocation()
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ei.GetName(), filepath.Base(loc.GetFile()), loc.GetLine(), e.GetReason()))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityError{EntityError: &protocol.EntityErrorEvent{
		Time:       timestamppb.Now(),
		EntityName: ei.GetName(),
		Error:      e,
	}}})
}

func (ew *eventWriter) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== end", ei.GetName()))
	}
	var skip *protocol.Skip
	if len(skipReasons) > 0 {
		skip = &protocol.Skip{Reasons: skipReasons}
	}
	tlpb, err := timingLog.Proto()
	if err != nil {
		return err
	}
	firstErr := ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityEnd{EntityEnd: &protocol.EntityEndEvent{
		Time:       timestamppb.Now(),
		EntityName: ei.GetName(),
		Skip:       skip,
		TimingLog:  tlpb,
	}}})
	// An entity in the current bundle is run. It means the output files are
	// already in the local directory, ready to be copied.
	if err := ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityCopyEnd{EntityCopyEnd: &protocol.EntityCopyEndEvent{
		EntityName: ei.GetName(),
	}}}); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (ew *eventWriter) ExternalEvent(req *protocol.RunTestsResponse) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	return ew.srv.Send(req)
}

func (ew *eventWriter) StackOperation(ctx context.Context, req *protocol.StackOperationRequest) (*protocol.StackOperationResponse, error) {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if err := ew.srv.Send(&protocol.RunTestsResponse{
		Type: &protocol.RunTestsResponse_StackOperation{
			StackOperation: req,
		},
	}); err != nil {
		return nil, err
	}
	resp, err := ew.srv.Recv()
	if err != nil {
		return nil, err
	}
	if _, ok := resp.Type.(*protocol.RunTestsRequest_StackOperationResponse); !ok {
		return nil, fmt.Errorf("unexpected return type %T", resp.Type)
	}
	return resp.GetStackOperationResponse(), nil
}

func (ew *eventWriter) Heartbeat() error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_Heartbeat{Heartbeat: &protocol.HeartbeatEvent{
		Time: timestamppb.Now(),
	}}})
}

// connectToTarget connects to the target DUT and returns its connection.
func connectToTarget(ctx context.Context, spec, keyFile, keyDir, proxyCommand string, beforeReboot func(context.Context, *dut.DUT) error) (_ *dut.DUT, retErr error) {
	// Do not attempt to connect to the target if we dont have it.
	if spec == "" || spec == "-" {
		return nil, nil
	}

	logging.Infof(ctx, "Connecting to DUT: %s", spec)
	dt, err := dut.New(spec, keyFile, keyDir, proxyCommand, beforeReboot)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %v", err)
	}
	defer func() {
		if retErr != nil {
			dt.Close(ctx)
		}
	}()

	if err := dt.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to DUT: %v", err)
	}

	return dt, nil
}

func defaultTempDir() (string, error) {
	tempBaseDir := filepath.Join(os.TempDir(), "tast/run_tmp")
	if err := os.MkdirAll(tempBaseDir, 0755); err != nil {
		return "", err
	}
	return os.MkdirTemp(tempBaseDir, "")
}

// prepareTempDir sets up tempDir to be used for the base temporary directory,
// and sets the TMPDIR environment variable to tempDir so that subsequent
// os.CreateTemp/MkdirTemp calls create temporary files under the directory.
// Returned function can be called to restore TMPDIR to the original value.
func prepareTempDir(tempDir string) (restore func(), err error) {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, command.NewStatusErrorf(statusError, "failed to create %s: %v", tempDir, err)
	}
	if err := os.Chmod(tempDir, 0777|os.ModeSticky); err != nil {
		return nil, command.NewStatusErrorf(statusError, "failed to chmod %s: %v", tempDir, err)
	}

	const envTempDir = "TMPDIR"
	oldTempDir, hasOldTempDir := os.LookupEnv(envTempDir)
	os.Setenv(envTempDir, tempDir)
	return func() {
		if hasOldTempDir {
			os.Setenv(envTempDir, oldTempDir)
		} else {
			os.Unsetenv(envTempDir)
		}
	}, nil
}

type stubFixture struct {
	setUpErrors []*protocol.Error
}

var _ testing.FixtureImpl = (*stubFixture)(nil)

func (sf *stubFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	for _, e := range sf.setUpErrors {
		s.Error(e.GetReason())
	}
	return nil
}

func (*stubFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}
func (*stubFixture) Reset(ctx context.Context) error                        { return nil }
func (*stubFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*stubFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

type connectionEnv struct {
	rd *testing.RemoteData
}

func (c *connectionEnv) close(ctx context.Context) {
	if c.rd == nil {
		return
	}
	logging.Info(ctx, "Disconnecting from DUT")
	// It is okay to ignore the error since we've finished testing at this point.
	if c.rd.DUT != nil {
		c.rd.DUT.Close(ctx)
	}

	for _, d := range c.rd.CompanionDUTs {
		if d != nil {
			d.Close(ctx)
		}
	}
}

// setUpConnection sets up a connection to a test bundle in another device bcfg
// specifies. Caller must call close after use.
func setUpConnection(ctx context.Context, scfg *StaticConfig, cfg *protocol.RunConfig, bcfg *protocol.BundleConfig) (_ *connectionEnv, retErr error) {
	pt := bcfg.GetPrimaryTarget()
	logging.Info(ctx, "Connecting to DUT")
	sshCfg := pt.GetDutConfig().GetSshConfig()
	dt, err := connectToTarget(ctx, sshCfg.GetConnectionSpec(), sshCfg.GetKeyFile(), sshCfg.GetKeyDir(), sshCfg.GetProxyCommand(), scfg.beforeReboot)
	if err != nil {
		return nil, command.NewStatusErrorf(statusError, "failed to connect to DUT: %v", err)
	}
	defer func() {
		if retErr != nil {
			dt.Close(ctx)
		}
	}()

	companionDUTs := make(map[string]*dut.DUT)
	for role, dut := range bcfg.GetCompanionDuts() {
		sshCfg := dut.GetSshConfig()
		d, err := connectToTarget(ctx, sshCfg.GetConnectionSpec(), sshCfg.GetKeyFile(), sshCfg.GetKeyDir(), sshCfg.GetProxyCommand(), scfg.beforeReboot)
		if err != nil {
			return nil, command.NewStatusErrorf(statusError, "failed to connect to companion DUT %v: %v", sshCfg.GetConnectionSpec(), err)
		}
		defer func() {
			if retErr != nil {
				d.Close(ctx)
			}
		}()
		companionDUTs[role] = d
	}
	// Copy information on files pushed by Tast to DUTs.
	var pushedFilesPaths = make(map[string]map[string]string)
	for _, pathsInfo := range cfg.GetPushedFilesInfo() {
		srcDsts := make(map[string]string)
		for src, dst := range pathsInfo.GetSrcDstPaths() {
			srcDsts[src] = dst
		}
		pushedFilesPaths[pathsInfo.Role] = srcDsts
	}
	return &connectionEnv{
		&testing.RemoteData{
			Meta: &testing.Meta{
				TastPath:         bcfg.GetMetaTestConfig().GetTastPath(),
				Target:           sshCfg.GetConnectionSpec(),
				RunFlags:         bcfg.GetMetaTestConfig().GetRunFlags(),
				ListFlags:        bcfg.GetMetaTestConfig().GetListFlags(),
				ConnectionSpec:   sshCfg.GetConnectionSpec(),
				PushedFilesPaths: pushedFilesPaths,
			},
			RPCHint:       testing.NewRPCHint(pt.GetBundleDir(), cfg.GetFeatures().GetInfra().GetVars()),
			DUT:           dt,
			CompanionDUTs: companionDUTs,
			KeyFile:       sshCfg.GetKeyFile(),
			KeyDir:        sshCfg.GetKeyDir(),
		},
	}, nil
}

type testEnv struct {
	tempDir        string
	removeTempDir  bool
	restoreTempDir func()

	postRunFunc func(context.Context) error
}

func (e *testEnv) close(ctx context.Context) error {
	var firstErr error

	if e.postRunFunc != nil {
		if err := e.postRunFunc(ctx); err != nil && firstErr == nil {
			firstErr = command.NewStatusErrorf(statusError, "post-run failed: %v", err)
		}
	}
	e.restoreTempDir()
	if e.removeTempDir {
		if err := os.RemoveAll(e.tempDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// setUpTestEnvironment sets up testing environment.
// It creates output/temp directories and runs runHooks.
// Caller must call close after use.
func setUpTestEnvironment(ctx context.Context, scfg *StaticConfig, cfg *protocol.RunConfig, bcfg *protocol.BundleConfig) (_ *testEnv, retErr error) {
	// Set up output directory.
	// OutDir can be empty if the caller is not interested in output
	// files, e.g. in unit tests.
	if outDir := cfg.GetDirs().GetOutDir(); outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create output directory")
		}
		// Call os.Chmod again to ensure permission. The mode passed to
		// os.MkdirAll is modified by umask, so we need an explicit chmod.
		if err := os.Chmod(outDir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to chmod output directory")
		}
	}

	// Set up temporary directory.
	tempDir := cfg.GetDirs().GetTempDir()
	var removeTempDir bool
	if tempDir == "" {
		var err error
		tempDir, err = defaultTempDir()
		if err != nil {
			return nil, err
		}
		removeTempDir = true
	}
	defer func() {
		if retErr != nil {
			if removeTempDir {
				os.RemoveAll(tempDir)
			}
		}
	}()

	restoreTempDir, err := prepareTempDir(tempDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			restoreTempDir()
		}
	}()

	var postRunFunc func(context.Context) error
	// Don't run runHook when remote fixtures are used.
	// The runHook for local bundles (ready.Wait) may reset the state remote
	// fixtures just have set up, e.g. enterprise enrollment.
	// TODO(crbug/1184567): consider long term plan about interactions between
	// remote fixtures and run hooks.
	if scfg.runHook != nil && cfg.GetStartFixtureState().GetName() == "" {
		var err error
		postRunFunc, err = scfg.runHook(ctx, cfg.GetSystemServicesTimeout().AsDuration())
		if err != nil {
			return nil, command.NewStatusErrorf(statusError, "pre-run failed: %v", err)
		}
	}
	return &testEnv{
		tempDir:        tempDir,
		removeTempDir:  removeTempDir,
		restoreTempDir: restoreTempDir,
		postRunFunc:    postRunFunc,
	}, nil
}
