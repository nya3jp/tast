// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log/syslog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/dut"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

const (
	statusSuccess     = 0 // bundle ran successfully
	statusError       = 1 // unclassified runtime error was encountered
	statusBadArgs     = 2 // bad command-line flags or other args were supplied
	statusBadTests    = 3 // errors in test registration (bad names, missing test functions, etc.)
	statusBadPatterns = 4 // one or more bad test patterns were passed to the bundle
	statusNoTests     = 5 // no tests were matched by the supplied patterns
)

// Delegate injects functions as a part of test bundle framework implementation.
type Delegate struct {
	// TestHook is called before each test in the test bundle if it is not nil.
	// The returned closure is executed after the test if it is not nil.
	TestHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)

	// RunHook is called at the beginning of a bundle execution if it is not nil.
	// The returned closure is executed at the end if it is not nil.
	// In case of errors, no test in the test bundle will run.
	RunHook func(context.Context) (func(context.Context) error, error)

	// Ready is called at the beginning of a bundle execution if it is not
	// nil and -waituntilready is set to true (default).
	// Local test bundles can specify a function to wait for the DUT to be
	// ready for tests to run. It is recommended to write informational
	// messages with testing.ContextLog to let the user know the reason for
	// the delay. In case of errors, no test in the test bundle will run.
	// This field has an effect only for local test bundles.
	Ready func(ctx context.Context) error

	// BeforeReboot is called before every reboot if it is not nil.
	// This field has an effect only for remote test bundles.
	BeforeReboot func(ctx context.Context, d *dut.DUT) error

	// BeforeDownload is called before the framework attempts to download
	// external data files if it is not nil.
	//
	// Test bundles can install this hook to recover from possible network
	// outage caused by previous tests. Note that it is called only when
	// the framework needs to download one or more external data files.
	//
	// Since no specific timeout is set to ctx, do remember to set a
	// reasonable timeout at the beginning of the hook to avoid blocking
	// for long time.
	BeforeDownload func(ctx context.Context)
}

// run reads a JSON-marshaled BundleArgs struct from stdin and performs the requested action.
// Default arguments may be specified via args, which will also be updated from stdin.
// The caller should exit with the returned status code.
func run(ctx context.Context, clArgs []string, stdin io.Reader, stdout, stderr io.Writer, scfg *staticConfig, bt bundleType) int {
	args, err := readArgs(clArgs, stdin, stderr, bt)
	if err != nil {
		return command.WriteError(stderr, err)
	}

	if errs := testing.RegistrationErrors(); len(errs) > 0 {
		es := make([]string, len(errs))
		for i, err := range errs {
			es[i] = err.Error()
		}
		err := command.NewStatusErrorf(statusBadTests, "error(s) in registered tests: %v", strings.Join(es, ", "))
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case jsonprotocol.BundleListTestsMode:
		tests, err := testsToRun(scfg, args.ListTests.Patterns)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		var infos []*jsonprotocol.EntityWithRunnabilityInfo
		features := args.ListTests.Features()
		for _, test := range tests {
			// If we encounter errors while checking test dependencies,
			// treat the test as not skipped. When we actually try to
			// run the test later, it will fail with errors.
			var skipReason string
			if reasons, err := test.Deps().Check(features); err == nil && len(reasons) > 0 {
				skipReason = strings.Join(append([]string(nil), reasons...), ", ")
			}
			infos = append(infos, &jsonprotocol.EntityWithRunnabilityInfo{
				EntityInfo: *jsonprotocol.MustEntityInfoFromProto(test.EntityProto()),
				SkipReason: skipReason,
			})
		}
		if err := json.NewEncoder(stdout).Encode(infos); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.BundleListFixturesMode:
		fixts := testing.GlobalRegistry().AllFixtures()
		var infos []*jsonprotocol.EntityInfo
		for _, f := range fixts {
			infos = append(infos, jsonprotocol.MustEntityInfoFromProto(f.EntityProto()))
		}
		sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
		if err := json.NewEncoder(stdout).Encode(infos); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.BundleExportMetadataMode:
		tests, err := testsToRun(scfg, nil)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := testing.WriteTestsAsProto(stdout, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.BundleRunTestsMode:
		tests, err := testsToRun(scfg, args.RunTests.Patterns)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := runTests(ctx, stdout, args, scfg, bt, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.BundleRPCMode:
		if err := RunRPCServer(stdin, stdout, testing.GlobalRegistry().AllServices()); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// testsToRun returns a sorted list of tests to run for the given patterns.
func testsToRun(scfg *staticConfig, patterns []string) ([]*testing.TestInstance, error) {
	m, err := testing.NewMatcher(patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", patterns, err.Error())
	}
	var tests []*testing.TestInstance
	for _, t := range testing.GlobalRegistry().AllTests() {
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

// staticConfig contains configurations unique to a test bundle.
//
// The supplied functions are used to provide customizations that apply to all
// entities in a test bundle. They may contain bundle-specific code.
type staticConfig struct {
	// runHook is run at the beginning of the entire series of tests if non-nil.
	// The returned closure is executed after the entire series of tests if not nil.
	runHook func(context.Context) (func(context.Context) error, error)
	// testHook is run before each test if non-nil.
	// If this function panics or reports errors, the precondition (if any)
	// will not be prepared and the test function will not run.
	// The returned closure is executed after a test if not nil.
	testHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)
	// beforeReboot is run before every reboot if non-nil.
	// The function must not call DUT.Reboot() or it will cause infinite recursion.
	beforeReboot func(context.Context, *dut.DUT) error
	// beforeDownload is run before downloading external data files if non-nil.
	beforeDownload func(context.Context)
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

func newStaticConfig(defaultTestTimeout time.Duration, d Delegate) *staticConfig {
	return &staticConfig{
		runHook: func(ctx context.Context) (func(context.Context) error, error) {
			pd, ok := testcontext.PrivateDataFromContext(ctx)
			if !ok {
				panic("BUG: PrivateData not available in run hook")
			}
			if d.Ready != nil && pd.WaitUntilReady {
				if err := d.Ready(ctx); err != nil {
					return nil, err
				}
			}
			if d.RunHook == nil {
				return nil, nil
			}
			return d.RunHook(ctx)
		},
		testHook:           d.TestHook,
		beforeReboot:       d.BeforeReboot,
		beforeDownload:     d.BeforeDownload,
		defaultTestTimeout: defaultTestTimeout,
	}
}

// eventWriter wraps MessageWriter to write events to syslog in parallel.
//
// eventWriter is goroutine-safe; it is safe to call its methods concurrently from multiple
// goroutines.
type eventWriter struct {
	mw *control.MessageWriter
	lg *syslog.Writer
}

func newEventWriter(mw *control.MessageWriter) *eventWriter {
	// Continue even if we fail to connect to syslog.
	lg, _ := syslog.New(syslog.LOG_INFO, "tast")
	return &eventWriter{mw: mw, lg: lg}
}

func (ew *eventWriter) RunLog(msg string) error {
	if ew.lg != nil {
		ew.lg.Info(msg)
	}
	return ew.mw.WriteMessage(&control.RunLog{Time: time.Now(), Text: msg})
}

func (ew *eventWriter) EntityStart(ei *protocol.Entity, outDir string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", ei.Name))
	}
	return ew.mw.WriteMessage(&control.EntityStart{Time: time.Now(), Info: *jsonprotocol.MustEntityInfoFromProto(ei), OutDir: outDir})
}

func (ew *eventWriter) EntityLog(ei *protocol.Entity, msg string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ei.Name, msg))
	}
	return ew.mw.WriteMessage(&control.EntityLog{Time: time.Now(), Text: msg, Name: ei.GetName()})
}

func (ew *eventWriter) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	if ew.lg != nil {
		loc := e.GetLocation()
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ei.GetName(), filepath.Base(loc.GetFile()), loc.GetLine(), e.GetReason()))
	}
	return ew.mw.WriteMessage(&control.EntityError{Time: time.Now(), Error: *jsonprotocol.ErrorFromProto(e), Name: ei.GetName()})
}

func (ew *eventWriter) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== end", ei.GetName()))
	}

	return ew.mw.WriteMessage(&control.EntityEnd{
		Time:        time.Now(),
		Name:        ei.Name,
		SkipReasons: skipReasons,
		TimingLog:   timingLog,
	})
}

// runTests runs tests per args and cfg and writes control messages to stdout.
//
// If an error is encountered in the test harness (as opposed to in a test), an error is returned.
// Otherwise, nil is returned (test errors will be reported via EntityError control messages).
func runTests(ctx context.Context, stdout io.Writer, args *jsonprotocol.BundleArgs, scfg *staticConfig,
	bt bundleType, tests []*testing.TestInstance) error {
	ctx = testcontext.WithPrivateData(ctx, testcontext.PrivateData{
		WaitUntilReady: args.RunTests.WaitUntilReady,
	})

	mw := control.NewMessageWriter(stdout)

	hbw := control.NewHeartbeatWriter(mw, args.RunTests.HeartbeatInterval)
	defer hbw.Stop()

	ew := newEventWriter(mw)
	ctx = testcontext.WithLogger(ctx, func(msg string) {
		ew.RunLog(msg)
	})

	if len(tests) == 0 {
		return command.NewStatusErrorf(statusNoTests, "no tests matched by pattern(s)")
	}

	if args.RunTests.TempDir == "" {
		var err error
		args.RunTests.TempDir, err = defaultTempDir()
		if err != nil {
			return err
		}
		defer os.RemoveAll(args.RunTests.TempDir)
	}

	restoreTempDir, err := prepareTempDir(args.RunTests.TempDir)
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
	if scfg.runHook != nil && args.RunTests.StartFixtureName == "" {
		var err error
		postRunFunc, err = scfg.runHook(ctx)
		if err != nil {
			return command.NewStatusErrorf(statusError, "pre-run failed: %v", err)
		}
	}

	var rd *testing.RemoteData
	if bt == remoteBundle {
		testcontext.Log(ctx, "Connecting to DUT")
		dt, err := connectToTarget(ctx, args.RunTests.Target, args.RunTests.KeyFile, args.RunTests.KeyDir, scfg.beforeReboot)
		if err != nil {
			return command.NewStatusErrorf(statusError, "failed to connect to DUT: %v", err)
		}
		defer func() {
			testcontext.Log(ctx, "Disconnecting from DUT")
			// It is okay to ignore the error since we've finished testing at this point.
			dt.Close(ctx)
		}()

		companionDUTs := make(map[string]*dut.DUT)
		defer func() {
			if len(companionDUTs) == 0 {
				return
			}
			testcontext.Log(ctx, "Disconnecting from companion DUTs")
			// It is okay to ignore the error since we've finished testing at this point.
			for _, dut := range rd.CompanionDUTs {
				dut.Close(ctx)
			}
		}()
		for role, addr := range args.RunTests.CompanionDUTs {
			dut, err := connectToTarget(ctx, addr, args.RunTests.KeyFile, args.RunTests.KeyDir, scfg.beforeReboot)
			if err != nil {
				return command.NewStatusErrorf(statusError, "failed to connect to companion DUT %v: %v", addr, err)
			}
			companionDUTs[role] = dut
		}

		rd = &testing.RemoteData{
			Meta: &testing.Meta{
				TastPath: args.RunTests.TastPath,
				Target:   args.RunTests.Target,
				RunFlags: args.RunTests.RunFlags,
			},
			RPCHint:       testing.NewRPCHint(args.RunTests.LocalBundleDir, args.RunTests.TestVars),
			DUT:           dt,
			CompanionDUTs: companionDUTs,
		}
	}

	pcfg := &planner.Config{
		DataDir:           args.RunTests.DataDir,
		OutDir:            args.RunTests.OutDir,
		Features:          args.RunTests.Features(),
		Devservers:        args.RunTests.Devservers,
		TLWServer:         args.RunTests.TLWServer,
		DUTName:           args.RunTests.DUTName,
		BuildArtifactsURL: args.RunTests.BuildArtifactsURL,
		RemoteData:        rd,
		TestHook:          scfg.testHook,
		DownloadMode:      args.RunTests.DownloadMode,
		BeforeDownload:    scfg.beforeDownload,
		Fixtures:          testing.GlobalRegistry().AllFixtures(),
		StartFixtureName:  args.RunTests.StartFixtureName,
		StartFixtureImpl:  &stubFixture{setUpErrors: args.RunTests.SetUpErrors},
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
	setUpErrors []string
}

var _ testing.FixtureImpl = (*stubFixture)(nil)

func (sf *stubFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	for _, e := range sf.setUpErrors {
		s.Error(e)
	}
	return nil
}

func (*stubFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}
func (*stubFixture) Reset(ctx context.Context) error                        { return nil }
func (*stubFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*stubFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
