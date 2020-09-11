// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
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
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/rpc"
	"chromiumos/tast/timing"
)

const (
	statusSuccess     = 0 // bundle ran successfully
	statusError       = 1 // unclassified runtime error was encountered
	statusBadArgs     = 2 // bad command-line flags or other args were supplied
	statusBadTests    = 3 // errors in test registration (bad names, missing test functions, etc.)
	statusBadPatterns = 4 // one or more bad test patterns were passed to the bundle
	statusNoTests     = 5 // no tests were matched by the supplied patterns
)

// run reads a JSON-marshaled Args struct from stdin and performs the requested action.
// Default arguments may be specified via args, which will also be updated from stdin.
// The caller should exit with the returned status code.
func run(ctx context.Context, clArgs []string, stdin io.Reader, stdout, stderr io.Writer,
	args *Args, cfg *runConfig, bt bundleType) int {
	if err := readArgs(clArgs, stdin, stderr, args, bt); err != nil {
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
	case ListTestsMode:
		tests, err := testsToRun(cfg, args.ListTests.Patterns)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		var infos []*testing.EntityInfo
		for _, test := range tests {
			infos = append(infos, test.EntityInfo())
		}
		if err := testing.WriteTestsAsJSON(stdout, infos); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case ListFixturesMode:
		// TODO(oka): Implement ListFixturesMode.
		panic("to be implemented")
	case ExportMetadataMode:
		tests, err := testsToRun(cfg, nil)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := testing.WriteTestsAsProto(stdout, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case RunTestsMode:
		tests, err := testsToRun(cfg, args.RunTests.Patterns)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := runTests(ctx, stdout, args, cfg, bt, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case RPCMode:
		if err := rpc.RunServer(stdin, stdout, testing.GlobalRegistry().AllServices()); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// testsToRun returns a sorted list of tests to run for the given patterns.
func testsToRun(cfg *runConfig, patterns []string) ([]*testing.TestInstance, error) {
	tests, err := testing.SelectTestsByArgs(testing.GlobalRegistry().AllTests(), patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", patterns, err.Error())
	}
	for _, tp := range tests {
		if tp.Timeout == 0 {
			tp.Timeout = cfg.defaultTestTimeout
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})
	return tests, nil
}

// runConfig contains additional parameters used when running tests.
//
// The supplied functions are used to provide customizations that apply to all local or all remote bundles
// and should not contain bundle-specific code (e.g. don't perform actions that depend on a UI being present,
// since some bundles may run on Chrome-OS-derived systems that don't contain Chrome). See ReadyFunc if
// bundle-specific work needs to be performed.
type runConfig struct {
	// runHook is run at the beginning of the entire series of tests if non-nil.
	// The returned closure is executed after the entire series of tests if not nil.
	runHook func(context.Context) (func(context.Context) error, error)
	// testHook is run before each test if non-nil.
	// If this function panics or reports errors, the precondition (if any)
	// will not be prepared and the test function will not run.
	// The returned closure is executed after a test if not nil.
	testHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
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

func (ew *eventWriter) EntityStart(ei *testing.EntityInfo, outDir string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", ei.Name))
	}
	return ew.mw.WriteMessage(&control.EntityStart{Time: time.Now(), Info: *ei, OutDir: outDir})
}

func (ew *eventWriter) EntityLog(ei *testing.EntityInfo, msg string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ei.Name, msg))
	}
	return ew.mw.WriteMessage(&control.EntityLog{Time: time.Now(), Text: msg, Name: ei.Name})
}

func (ew *eventWriter) EntityError(ei *testing.EntityInfo, e *testing.Error) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ei.Name, filepath.Base(e.File), e.Line, e.Reason))
	}
	return ew.mw.WriteMessage(&control.EntityError{Time: time.Now(), Error: *e, Name: ei.Name})
}

func (ew *eventWriter) EntityEnd(ei *testing.EntityInfo, skipReasons []string, timingLog *timing.Log) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== end", ei.Name))
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
func runTests(ctx context.Context, stdout io.Writer, args *Args, cfg *runConfig,
	bt bundleType, tests []*testing.TestInstance) error {
	mw := control.NewMessageWriter(stdout)

	hbw := control.NewHeartbeatWriter(mw, args.RunTests.HeartbeatInterval)
	defer hbw.Stop()

	ew := newEventWriter(mw)
	ctx = logging.NewContext(ctx, func(msg string) {
		ew.RunLog(msg)
	})

	if len(tests) == 0 {
		return command.NewStatusErrorf(statusNoTests, "no tests matched by pattern(s)")
	}

	if args.RunTests.TempDir == "" {
		tempBaseDir := filepath.Join(os.TempDir(), "tast/run_tmp")
		if err := os.MkdirAll(tempBaseDir, 0755); err != nil {
			return err
		}

		var err error
		args.RunTests.TempDir, err = ioutil.TempDir(tempBaseDir, "")
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
	if cfg.runHook != nil {
		var err error
		postRunFunc, err = cfg.runHook(ctx)
		if err != nil {
			return command.NewStatusErrorf(statusError, "pre-run failed: %v", err)
		}
	}

	var rd *testing.RemoteData
	if bt == remoteBundle {
		logging.ContextLog(ctx, "Connecting to DUT")
		dt, err := connectToTarget(ctx, args)
		if err != nil {
			return command.NewStatusErrorf(statusError, "failed to connect to DUT: %v", err)
		}
		defer func() {
			logging.ContextLog(ctx, "Disconnecting from DUT")
			// It is okay to ignore the error since we've finished testing at this point.
			dt.Close(ctx)
		}()

		rd = &testing.RemoteData{
			Meta: &testing.Meta{
				TastPath: args.RunTests.TastPath,
				Target:   args.RunTests.Target,
				RunFlags: args.RunTests.RunFlags,
			},
			RPCHint: &testing.RPCHint{
				LocalBundleDir: args.RunTests.LocalBundleDir,
			},
			DUT: dt,
		}
	}

	pcfg := &planner.Config{
		DataDir:           args.RunTests.DataDir,
		OutDir:            args.RunTests.OutDir,
		Vars:              args.RunTests.TestVars,
		Features:          *args.RunTests.Features(),
		Devservers:        args.RunTests.Devservers,
		TLWServer:         args.RunTests.TLWServer,
		DUTName:           args.RunTests.DUTName,
		BuildArtifactsURL: args.RunTests.BuildArtifactsURL,
		RemoteData:        rd,
		TestHook:          cfg.testHook,
		DownloadMode:      args.RunTests.DownloadMode,
		Fixtures:          testing.GlobalRegistry().AllFixtures(),
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
func connectToTarget(ctx context.Context, args *Args) (_ *dut.DUT, retErr error) {
	if args.RunTests.Target == "" {
		return nil, errors.New("target not supplied")
	}

	dt, err := dut.New(args.RunTests.Target, args.RunTests.KeyFile, args.RunTests.KeyDir)
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

// lockStdIO replaces os.Stdin, os.Stdout and os.Stderr with closed pipes and
// returns the original files. This function can be called at the beginning of
// test bundles to ensure that calling fmt.Print and its family does not corrupt
// the control channel.
func lockStdIO() (stdin, stdout, stderr *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprint("Failed to lock stdio: ", err))
	}
	r.Close()
	w.Close()

	stdin = os.Stdin
	stdout = os.Stdout
	stderr = os.Stderr
	os.Stdin = r
	os.Stdout = w
	os.Stderr = w
	return stdin, stdout, stderr
}
