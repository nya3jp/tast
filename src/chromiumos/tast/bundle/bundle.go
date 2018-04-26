// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
	tests, err := readArgs(stdin, args, bt)
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

// runConfig contains additional parameters used by runTests.
type runConfig struct {
	// runSetupFunc is run at the beginning of the entire test run if non-nil.
	// ctx (or a derived context with additional values) should be returned by the function.
	runSetupFunc func(ctx context.Context) (context.Context, error)
	// runCleanupFunc is run at the end of the entire test run if non-nil.
	runCleanupFunc func(ctx context.Context) error
	// testSetupFunc is run before each test if non-nil.
	testSetupFunc func(ctx context.Context) error
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

	if len(tests) == 0 {
		return command.NewStatusErrorf(statusNoTests, "no tests matched by pattern(s)")
	}

	if cfg.runSetupFunc != nil {
		var err error
		if ctx, err = cfg.runSetupFunc(ctx); err != nil {
			return command.NewStatusErrorf(statusError, "run setup failed: %v", err)
		}
	}

	for _, t := range tests {
		// Make a copy of the test with the default timeout if none was specified.
		test := *t
		if test.Timeout == 0 {
			test.Timeout = cfg.defaultTestTimeout
		}

		mw.WriteMessage(&control.TestStart{Time: time.Now(), Test: test})

		outDir := filepath.Join(args.OutDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return command.NewStatusErrorf(statusError, "failed to create output dir: %v", err)
		}

		if cfg.testSetupFunc != nil {
			if err := cfg.testSetupFunc(ctx); err != nil {
				return command.NewStatusErrorf(statusError, "test setup failed: %v", err)
			}
		}
		ch := make(chan testing.Output)
		s := testing.NewState(ctx, ch, filepath.Join(args.DataDir, test.DataDir()), outDir,
			test.Timeout, test.CleanupTimeout)

		done := make(chan bool, 1)
		go func() {
			copyTestOutput(ch, mw)
			done <- true
		}()
		test.Run(s)
		close(ch)
		<-done

		mw.WriteMessage(&control.TestEnd{Time: time.Now(), Name: test.Name})
	}

	if cfg.runCleanupFunc != nil {
		if err := cfg.runCleanupFunc(ctx); err != nil {
			return command.NewStatusErrorf(statusError, "run cleanup failed: %v", err)
		}
	}
	return nil
}

// copyTestOutput reads test output from ch and writes it to mw.
func copyTestOutput(ch chan testing.Output, mw *control.MessageWriter) {
	for o := range ch {
		if o.Err != nil {
			mw.WriteMessage(&control.TestError{Time: o.T, Error: *o.Err})
		} else {
			mw.WriteMessage(&control.TestLog{Time: o.T, Text: o.Msg})
		}
	}
}
