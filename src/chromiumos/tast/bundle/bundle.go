// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/command"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
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
func run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer,
	args *Args, cfg *runConfig, bt bundleType) int {
	tests, err := readArgs(stdin, args, cfg, bt)
	if err != nil {
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case ListTestsMode:
		if err := testing.WriteTestsAsJSON(stdout, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case RunTestsMode:
		if err := runTests(ctx, stdout, args, cfg, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// logFunc can be called by functions registered in runConfig to log a message.
type logFunc func(msg string)

// runConfig contains additional parameters used when running tests.
type runConfig struct {
	// runSetupFunc is run at the beginning of the entire test run if non-nil.
	// The provided context (or a derived context with additional values) should be returned by the function.
	runSetupFunc func(context.Context, logFunc) (context.Context, error)
	// runCleanupFunc is run at the end of the entire test run if non-nil.
	runCleanupFunc func(context.Context, logFunc) error
	// testSetupFunc is run before each test if non-nil.
	// If this function panicked or reported errors, neither the test body function nor testCleanupFunc will run.
	testSetupFunc func(*testing.State)
	// testCleanFunc is run at the end of each test if non-nil.
	// If testSetupFunc panicked or reported errors, this will not run.
	testCleanupFunc func(*testing.State)
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// runTests runs tests per args and cfg and writes control messages to stdout.
//
// If an error is encountered in the test harness (as opposed to in a test), an error is returned.
// Otherwise, nil is returned (test errors will be reported via TestError control messages).
func runTests(ctx context.Context, stdout io.Writer, args *Args, cfg *runConfig,
	tests []*testing.Test) error {
	mw := control.NewMessageWriter(stdout)
	lf := func(msg string) { mw.WriteMessage(&control.RunLog{Time: time.Now(), Text: msg}) }

	if len(tests) == 0 {
		return command.NewStatusErrorf(statusNoTests, "no tests matched by pattern(s)")
	}

	if cfg.runSetupFunc != nil {
		var err error
		if ctx, err = cfg.runSetupFunc(ctx, lf); err != nil {
			return command.NewStatusErrorf(statusError, "run setup failed: %v", err)
		}
	}

	var meta *testing.Meta
	if !reflect.DeepEqual(args.RemoteArgs, RemoteArgs{}) {
		meta = &testing.Meta{
			TastPath: args.RemoteArgs.TastPath,
			Target:   args.RemoteArgs.Target,
			RunFlags: args.RemoteArgs.RunFlags,
		}
	}

	for _, t := range tests {
		if err := runTest(ctx, mw, args, cfg, t, meta); err != nil {
			return err
		}
	}

	if cfg.runCleanupFunc != nil {
		if err := cfg.runCleanupFunc(ctx, lf); err != nil {
			return command.NewStatusErrorf(statusError, "run cleanup failed: %v", err)
		}
	}
	return nil
}

// runTest runs t per args and cfg, writing the appropriate control.Test* control messages to mw.
func runTest(ctx context.Context, mw *control.MessageWriter, args *Args, cfg *runConfig,
	t *testing.Test, meta *testing.Meta) error {
	mw.WriteMessage(&control.TestStart{
		Time: time.Now(),
		Test: *t,
	})

	// We skip running the test if it has any dependencies on software features that aren't
	// provided by the DUT, but we additionally report an error if one or more dependencies
	// refer to features that we don't know anything about (possibly indicating a typo in the
	// test's dependencies).
	var missingDeps []string
	if args.CheckSoftwareDeps {
		missingDeps = t.MissingSoftwareDeps(args.AvailableSoftwareFeatures)
		if unknown := getUnknownDeps(missingDeps, args); len(unknown) > 0 {
			_, fn, ln, _ := runtime.Caller(0)
			mw.WriteMessage(&control.TestError{
				Time: time.Now(),
				Error: testing.Error{
					Reason: "Unknown dependencies: " + strings.Join(unknown, " "),
					File:   fn,
					Line:   ln,
				},
			})
		}
	}

	if len(missingDeps) == 0 {
		testCfg := testing.TestConfig{
			DataDir:     filepath.Join(args.DataDir, t.DataDir()),
			OutDir:      filepath.Join(args.OutDir, t.Name),
			Meta:        meta,
			SetupFunc:   cfg.testSetupFunc,
			CleanupFunc: cfg.testCleanupFunc,
		}

		ch := make(chan testing.Output)
		abortCopier := make(chan bool, 1)
		copierDone := make(chan bool, 1)

		// Copy test output in the background as soon as it becomes available.
		go func() {
			copyTestOutput(ch, mw, abortCopier)
			copierDone <- true
		}()

		if !t.Run(ctx, ch, &testCfg) {
			// If Run reported that the test didn't finish, tell the copier to abort.
			abortCopier <- true
		}
		<-copierDone
	}

	mw.WriteMessage(&control.TestEnd{
		Time:                time.Now(),
		Name:                t.Name,
		MissingSoftwareDeps: missingDeps,
	})
	return nil
}

// getUnknownDeps returns a sorted list of software dependencies from missingDeps that
// aren't referring to known features.
func getUnknownDeps(missingDeps []string, args *Args) []string {
	var unknown []string
DepsLoop:
	for _, d := range missingDeps {
		for _, f := range args.UnavailableSoftwareFeatures {
			if d == f {
				continue DepsLoop
			}
		}
		unknown = append(unknown, d)
	}
	sort.Strings(unknown)
	return unknown
}

// copyTestOutput reads test output from ch and writes it to mw until ch is closed.
// If abort becomes readable before ch is closed, a timeout error is written to mw
// and the function returns immediately.
func copyTestOutput(ch chan testing.Output, mw *control.MessageWriter, abort chan bool) {
	for {
		select {
		case o, ok := <-ch:
			if !ok {
				// Channel was closed, i.e. test finished.
				return
			}
			if o.Err != nil {
				mw.WriteMessage(&control.TestError{Time: o.T, Error: *o.Err})
			} else {
				mw.WriteMessage(&control.TestLog{Time: o.T, Text: o.Msg})
			}
		case <-abort:
			mw.WriteMessage(&control.TestError{
				Time:  time.Now(),
				Error: *testing.NewError("Test timed out", 0),
			})
			return
		}
	}
}
