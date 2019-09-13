// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log/syslog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/command"
	"chromiumos/tast/control"
	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
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
		if err := testing.WriteTestsAsJSON(stdout, tests); err != nil {
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
func testsToRun(cfg *runConfig, patterns []string) ([]*testing.TestCase, error) {
	tests, err := testing.SelectTestsByArgs(testing.GlobalRegistry().AllTests(), patterns)
	if err != nil {
		return nil, command.NewStatusErrorf(statusBadPatterns, "failed getting tests for %v: %v", patterns, err.Error())
	}
	for _, tp := range tests {
		if tp.Timeout == 0 {
			tp.Timeout = cfg.defaultTestTimeout
		}
	}
	testing.SortTests(tests)
	return tests, nil
}

// logFunc can be called by functions registered in runConfig to log a message.
// It is safe to call it concurrently from different goroutines.
type logFunc func(msg string)

// runConfig contains additional parameters used when running tests.
//
// The supplied functions are used to provide customizations that apply to all local or all remote bundles
// and should not contain bundle-specific code (e.g. don't perform actions that depend on a UI being present,
// since some bundles may run on Chrome-OS-derived systems that don't contain Chrome). See ReadyFunc if
// bundle-specific work needs to be performed.
type runConfig struct {
	// preRunFunc is run at the beginning of the entire series of tests if non-nil.
	// The provided context (or a derived context with additional values) should be returned by the function.
	preRunFunc func(context.Context, logFunc) (context.Context, error)
	// postRunFunc is run at the end of the entire series of tests if non-nil.
	postRunFunc func(context.Context, logFunc) error
	// preTestFunc is run before each test if non-nil.
	// If this function panics or reports errors, the precondition (if any)
	// will not be prepared and the test function will not run.
	preTestFunc func(context.Context, *testing.State)
	// postTestFunc is run unconditionally at the end of each test if non-nil.
	postTestFunc func(context.Context, *testing.State)
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// eventWriter is used to report test events.
//
// eventWriter is goroutine-safe; it is safe to call its methods concurrently from multiple
// goroutines.
//
// Events are basically written through to MessageWriter, but they are also sent to syslog for
// easier debugging.
type eventWriter struct {
	mw *control.MessageWriter
	lg *syslog.Writer

	testName string // name of the current test
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

func (ew *eventWriter) TestStart(t *testing.TestCase) error {
	ew.testName = t.Name
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== start", t.Name))
	}
	return ew.mw.WriteMessage(&control.TestStart{Time: time.Now(), Test: *t})
}

func (ew *eventWriter) TestLog(ts time.Time, msg string) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: %s", ew.testName, msg))
	}
	return ew.mw.WriteMessage(&control.TestLog{Time: ts, Text: msg})
}

func (ew *eventWriter) TestError(ts time.Time, e *testing.Error) error {
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: Error at %s:%d: %s", ew.testName, filepath.Base(e.File), e.Line, e.Reason))
	}
	return ew.mw.WriteMessage(&control.TestError{Time: ts, Error: *e})
}

func (ew *eventWriter) TestEnd(t *testing.TestCase, missingDeps []string, timingLog *timing.Log) error {
	ew.testName = ""
	if ew.lg != nil {
		ew.lg.Info(fmt.Sprintf("%s: ======== end", t.Name))
	}

	return ew.mw.WriteMessage(&control.TestEnd{
		Time:                time.Now(),
		Name:                t.Name,
		MissingSoftwareDeps: missingDeps,
		TimingLog:           timingLog,
	})
}

// runTests runs tests per args and cfg and writes control messages to stdout.
//
// If an error is encountered in the test harness (as opposed to in a test), an error is returned.
// Otherwise, nil is returned (test errors will be reported via TestError control messages).
func runTests(ctx context.Context, stdout io.Writer, args *Args, cfg *runConfig,
	bt bundleType, tests []*testing.TestCase) error {
	mw := control.NewMessageWriter(stdout)

	hbw := control.NewHeartbeatWriter(mw, args.RunTests.HeartbeatInterval)
	defer hbw.Stop()

	ew := newEventWriter(mw)

	lf := func(msg string) {
		ew.RunLog(msg)
	}

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

	if cfg.preRunFunc != nil {
		var err error
		if ctx, err = cfg.preRunFunc(ctx, lf); err != nil {
			return command.NewStatusErrorf(statusError, "pre-run failed: %v", err)
		}
	}

	var meta *testing.Meta
	if bt == remoteBundle {
		meta = &testing.Meta{
			TastPath: args.RunTests.TastPath,
			Target:   args.RunTests.Target,
			RunFlags: args.RunTests.RunFlags,
		}
	}

	// Make a backward pass through the tests, checking if each will be run (i.e. dependencies are satisfied).
	// As we go, build up a slice of the next test that will actually be run after each test.
	// We pass this information to runTest later to ensure that we don't incorrectly fail
	// to close a precondition if the final test using the precondition is skipped: https://crbug.com/950499
	var rt *testing.TestCase                           // last-seen test that will be run
	missingDeps := make([][]string, len(tests))        // per-test missing deps, in same order as tests
	nextTests := make([]*testing.TestCase, len(tests)) // test to run after each test, in same order as tests
	for i := len(tests) - 1; i >= 0; i-- {
		nextTests[i] = rt
		if args.RunTests.CheckSoftwareDeps {
			missingDeps[i] = tests[i].MissingSoftwareDeps(args.RunTests.AvailableSoftwareFeatures)
		}
		if len(missingDeps[i]) == 0 {
			rt = tests[i]
		}
	}

	for i, t := range tests {
		if md := missingDeps[i]; len(md) == 0 {
			runTest(ctx, ew, args, cfg, t, nextTests[i], meta)
		} else {
			reportSkippedTest(ctx, ew, args, t, md)
		}
	}

	if cfg.postRunFunc != nil {
		if err := cfg.postRunFunc(ctx, lf); err != nil {
			return command.NewStatusErrorf(statusError, "post-run failed: %v", err)
		}
	}
	return nil
}

// runTest runs t per args and cfg, writing the appropriate control.Test* control messages to mw.
func runTest(ctx context.Context, ew *eventWriter, args *Args, cfg *runConfig,
	t, next *testing.TestCase, meta *testing.Meta) {
	ew.TestStart(t)

	// Attach a log that the test can use to report timing events.
	timingLog := timing.NewLog()
	ctx = timing.NewContext(ctx, timingLog)

	testCfg := testing.TestConfig{
		DataDir:      filepath.Join(args.RunTests.DataDir, t.DataDir()),
		OutDir:       filepath.Join(args.RunTests.OutDir, t.Name),
		Vars:         args.RunTests.TestVars,
		Meta:         meta,
		PreTestFunc:  cfg.preTestFunc,
		PostTestFunc: cfg.postTestFunc,
		NextTest:     next,
	}

	ch := make(chan testing.Output)
	abortCopier := make(chan bool, 1)
	copierDone := make(chan bool, 1)

	// Copy test output in the background as soon as it becomes available.
	go func() {
		copyTestOutput(ch, ew, abortCopier)
		copierDone <- true
	}()

	if !t.Run(ctx, ch, &testCfg) {
		// If Run reported that the test didn't finish, tell the copier to abort.
		abortCopier <- true
	}
	<-copierDone

	ew.TestEnd(t, nil, timingLog)
}

// reportSkippedTest is called instead of runTest for a test that is skipped due to
// having unsatisfied dependencies.
func reportSkippedTest(ctx context.Context, ew *eventWriter, args *Args,
	t *testing.TestCase, missingDeps []string) {
	ew.TestStart(t)

	// Additionally report an error if one or more dependencies refer to features that
	// we don't know anything about (possibly indicating a typo in the test's dependencies).
	if unknown := getUnknownDeps(missingDeps, args); len(unknown) > 0 {
		_, fn, ln, _ := runtime.Caller(0)
		ew.TestError(time.Now(), &testing.Error{
			Reason: "Unknown dependencies: " + strings.Join(unknown, " "),
			File:   fn,
			Line:   ln,
		})
	}

	ew.TestEnd(t, missingDeps, nil)
}

// getUnknownDeps returns a sorted list of software dependencies from missingDeps that
// aren't referring to known features.
func getUnknownDeps(missingDeps []string, args *Args) []string {
	var unknown []string
DepsLoop:
	for _, d := range missingDeps {
		for _, f := range args.RunTests.UnavailableSoftwareFeatures {
			if d == f {
				continue DepsLoop
			}
		}
		unknown = append(unknown, d)
	}
	sort.Strings(unknown)
	return unknown
}

// copyTestOutput reads test output from ch and writes it to ew until ch is closed.
// If abort becomes readable before ch is closed, a timeout error is written to ew
// and the function returns immediately.
func copyTestOutput(ch <-chan testing.Output, ew *eventWriter, abort <-chan bool) {
	for {
		select {
		case o, ok := <-ch:
			if !ok {
				// Channel was closed, i.e. test finished.
				return
			}
			if o.Err != nil {
				ew.TestError(o.T, o.Err)
			} else {
				ew.TestLog(o.T, o.Msg)
			}
		case <-abort:
			const msg = "Test timed out"
			ew.TestError(time.Now(), testing.NewError(nil, msg, msg, 0))
			return
		}
	}
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
