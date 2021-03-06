// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
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
func runTests(ctx context.Context, srv protocol.TestService_RunTestsServer, cfg *protocol.RunConfig, scfg *StaticConfig) error {
	ctx = testcontext.WithPrivateData(ctx, testcontext.PrivateData{
		WaitUntilReady: cfg.GetWaitUntilReady(),
	})

	ew := newEventWriter(srv)

	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) { ew.RunLog(msg) }))
	ctx = logging.AttachLogger(ctx, logger)

	tests, err := testsToRun(scfg, cfg.GetTests())
	if err != nil {
		return err
	}

	// Set up output directory.
	// OutDir can be empty if the caller is not interested in output
	// files, e.g. in unit tests.
	if outDir := cfg.GetDirs().GetOutDir(); outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return errors.Wrap(err, "failed to create output directory")
		}
		// Call os.Chmod again to ensure permission. The mode passed to
		// os.MkdirAll is modified by umask, so we need an explicit chmod.
		if err := os.Chmod(outDir, 0755); err != nil {
			return errors.Wrap(err, "failed to chmod output directory")
		}
	}

	// Set up temporary directory.
	tempDir := cfg.GetDirs().GetTempDir()
	if tempDir == "" {
		var err error
		tempDir, err = defaultTempDir()
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
	}

	restoreTempDir, err := prepareTempDir(tempDir)
	if err != nil {
		return err
	}
	defer restoreTempDir()

	var postRunFunc func(context.Context) error

	// Don't run runHook when remote fixtures are used.
	// The runHook for local bundles (ready.Wait) may reset the state remote
	// fixtures just have set up, e.g. enterprise enrollment.
	// TODO(crbug/1184567): consider long term plan about interactions between
	// remote fixtures and run hooks.
	if scfg.runHook != nil && cfg.GetStartFixtureState().GetName() == "" {
		var err error
		postRunFunc, err = scfg.runHook(ctx)
		if err != nil {
			return command.NewStatusErrorf(statusError, "pre-run failed: %v", err)
		}
	}

	var rd *testing.RemoteData
	if rcfg := cfg.GetRemoteTestConfig(); rcfg != nil {
		logging.Info(ctx, "Connecting to DUT")
		sshCfg := rcfg.GetPrimaryDut().GetSshConfig()
		dt, err := connectToTarget(ctx, sshCfg.GetTarget(), sshCfg.GetKeyFile(), sshCfg.GetKeyDir(), scfg.beforeReboot)
		if err != nil {
			return command.NewStatusErrorf(statusError, "failed to connect to DUT: %v", err)
		}
		defer func() {
			logging.Info(ctx, "Disconnecting from DUT")
			// It is okay to ignore the error since we've finished testing at this point.
			dt.Close(ctx)
		}()

		companionDUTs := make(map[string]*dut.DUT)
		defer func() {
			if len(companionDUTs) == 0 {
				return
			}
			logging.Info(ctx, "Disconnecting from companion DUTs")
			// It is okay to ignore the error since we've finished testing at this point.
			for _, dut := range rd.CompanionDUTs {
				dut.Close(ctx)
			}
		}()
		for role, dut := range rcfg.GetCompanionDuts() {
			sshCfg := dut.GetSshConfig()
			d, err := connectToTarget(ctx, sshCfg.GetTarget(), sshCfg.GetKeyFile(), sshCfg.GetKeyDir(), scfg.beforeReboot)
			if err != nil {
				return command.NewStatusErrorf(statusError, "failed to connect to companion DUT %v: %v", sshCfg.GetTarget(), err)
			}
			companionDUTs[role] = d
		}

		rd = &testing.RemoteData{
			Meta: &testing.Meta{
				TastPath: cfg.GetRemoteTestConfig().GetMetaTestConfig().GetTastPath(),
				Target:   sshCfg.GetTarget(),
				RunFlags: cfg.GetRemoteTestConfig().GetMetaTestConfig().GetRunFlags(),
			},
			RPCHint:       testing.NewRPCHint(cfg.GetRemoteTestConfig().GetLocalBundleDir(), cfg.GetFeatures().GetInfra().GetVars()),
			DUT:           dt,
			CompanionDUTs: companionDUTs,
		}
	}

	mode, err := planner.DownloadModeFromProto(cfg.GetDataFileConfig().GetDownloadMode())
	if err != nil {
		return command.NewStatusErrorf(statusError, "%v", err)
	}

	pcfg := &planner.Config{
		DataDir:           cfg.GetDirs().GetDataDir(),
		OutDir:            cfg.GetDirs().GetOutDir(),
		Features:          cfg.GetFeatures(),
		Devservers:        cfg.GetServiceConfig().GetDevservers(),
		TLWServer:         cfg.GetServiceConfig().GetTlwServer(),
		DUTName:           cfg.GetServiceConfig().GetTlwSelfName(),
		BuildArtifactsURL: cfg.GetDataFileConfig().GetBuildArtifactsUrl(),
		RemoteData:        rd,
		TestHook:          scfg.testHook,
		DownloadMode:      mode,
		BeforeDownload:    scfg.beforeDownload,
		Fixtures:          scfg.registry.AllFixtures(),
		StartFixtureName:  cfg.GetStartFixtureState().GetName(),
		StartFixtureImpl:  &stubFixture{setUpErrors: cfg.GetStartFixtureState().GetErrors()},
	}

	if err := planner.RunTests(ctx, tests, ew, pcfg); err != nil {
		return command.NewStatusErrorf(statusError, "run failed: %v", err)
	}

	if postRunFunc != nil {
		if err := postRunFunc(ctx); err != nil {
			return command.NewStatusErrorf(statusError, "post-run failed: %v", err)
		}
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
}

func newEventWriter(srv protocol.TestService_RunTestsServer) *eventWriter {
	// Continue even if we fail to connect to syslog.
	lg, _ := syslog.New(syslog.LOG_INFO, "tast")
	return &eventWriter{srv: srv, lg: lg}
}

func (ew *eventWriter) RunLog(msg string) error {
	if ew.lg != nil {
		ew.lg.Info(msg)
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_RunLog{RunLog: &protocol.RunLogEvent{
		Time: ptypes.TimestampNow(),
		Text: msg,
	}}})
}

func (ew *eventWriter) EntityStart(ei *protocol.Entity, outDir string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", ei.Name))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityStart{EntityStart: &protocol.EntityStartEvent{
		Time:   ptypes.TimestampNow(),
		Entity: ei,
		OutDir: outDir,
	}}})
}

func (ew *eventWriter) EntityLog(ei *protocol.Entity, msg string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ei.Name, msg))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityLog{EntityLog: &protocol.EntityLogEvent{
		Time:       ptypes.TimestampNow(),
		EntityName: ei.GetName(),
		Text:       msg,
	}}})
}

func (ew *eventWriter) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	if ew.lg != nil {
		loc := e.GetLocation()
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ei.GetName(), filepath.Base(loc.GetFile()), loc.GetLine(), e.GetReason()))
	}
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityError{EntityError: &protocol.EntityErrorEvent{
		Time:       ptypes.TimestampNow(),
		EntityName: ei.GetName(),
		Error:      e,
	}}})
}

func (ew *eventWriter) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
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
	return ew.srv.Send(&protocol.RunTestsResponse{Type: &protocol.RunTestsResponse_EntityEnd{EntityEnd: &protocol.EntityEndEvent{
		Time:       ptypes.TimestampNow(),
		EntityName: ei.GetName(),
		Skip:       skip,
		TimingLog:  tlpb,
	}}})
}

// connectToTarget connects to the target DUT and returns its connection.
func connectToTarget(ctx context.Context, target, keyFile, keyDir string, beforeReboot func(context.Context, *dut.DUT) error) (_ *dut.DUT, retErr error) {
	if target == "" {
		return nil, errors.New("target not supplied")
	}

	dt, err := dut.New(target, keyFile, keyDir, beforeReboot)
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
	return ioutil.TempDir(tempBaseDir, "")
}

// prepareTempDir sets up tempDir to be used for the base temporary directory,
// and sets the TMPDIR environment variable to tempDir so that subsequent
// ioutil.TempFile/TempDir calls create temporary files under the directory.
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
