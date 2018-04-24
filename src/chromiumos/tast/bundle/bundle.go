// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
// Default arguments and parameters may be specified via args and cfg.
// The caller should exit with the returned status code.
func run(ctx context.Context, stdin io.Reader, stdout io.Writer,
	args *Args, cfg *runConfig, bt bundleType) int {
	if st := readArgs(stdin, stdout, args, cfg, bt); st != statusSuccess {
		return st
	}

	switch args.Mode {
	case ListTestsMode:
		if err := testing.WriteTestsAsJSON(stdout, cfg.tests); err != nil {
			writeError(err.Error())
			return statusError
		}
		return statusSuccess
	case RunTestsMode:
		return runTests(ctx, args, cfg)
	default:
		writeError(fmt.Sprintf("Invalid mode %v", args.Mode))
		return statusBadArgs
	}
}

// writeError writes an error to stderr.
func writeError(msg string) {
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg += "\n"
	}
	io.WriteString(os.Stderr, msg)
}

// runConfig describes how runTests should run tests.
type runConfig struct {
	// mw is used to send control messages to the controlling process.
	// It is initialized by readArgs.
	mw *control.MessageWriter
	// tests contains tests to run. It is initialized by readArgs.
	tests []*testing.Test

	// runSetupFunc is run at the begining of the entire test run if non-nil.
	// ctx is the context supplied to the run function. It should be returned by the
	// function (possibly after additional values have been attached to it).
	runSetupFunc func(ctx context.Context) (context.Context, error)
	// runCleanupFunc is run at the end of the entire test run if non-nil.
	runCleanupFunc func(ctx context.Context) error
	// testSetupFunc is run before each test if non-nil.
	testSetupFunc func(ctx context.Context) error
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// runTests runs tests per args and cfg and writes control messages to cfg.stdout.
// The caller should exit with the returned status code.
//
// If an error is encountered in the test harness (as opposed to in a test), a human-readable
// message is written to stderr and a nonzero status is returned. Otherwise, statusSuccess is
// returned (test errors will be reported via TestError control messages).
func runTests(ctx context.Context, args *Args, cfg *runConfig) int {
	if len(cfg.tests) == 0 {
		writeError("No tests matched by pattern(s)")
		return statusNoTests
	}

	if cfg.runSetupFunc != nil {
		var err error
		if ctx, err = cfg.runSetupFunc(ctx); err != nil {
			writeError("Run setup failed: " + err.Error())
			return statusError
		}
	}

	for _, t := range cfg.tests {
		// Make a copy of the test with the default timeout if none was specified.
		test := *t
		if test.Timeout == 0 {
			test.Timeout = cfg.defaultTestTimeout
		}

		cfg.mw.WriteMessage(&control.TestStart{Time: time.Now(), Test: test})

		outDir := filepath.Join(args.OutDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			writeError("Failed to create output dir: " + err.Error())
			return statusError
		}

		if cfg.testSetupFunc != nil {
			if err := cfg.testSetupFunc(ctx); err != nil {
				writeError("Test setup failed: " + err.Error())
				return statusError
			}
		}
		ch := make(chan testing.Output)
		s := testing.NewState(ctx, ch, filepath.Join(args.DataDir, test.DataDir()), outDir,
			test.Timeout, test.CleanupTimeout)

		done := make(chan bool, 1)
		go func() {
			copyTestOutput(ch, cfg.mw)
			done <- true
		}()
		test.Run(s)
		close(ch)
		<-done

		cfg.mw.WriteMessage(&control.TestEnd{Time: time.Now(), Name: test.Name})
	}

	if cfg.runCleanupFunc != nil {
		if err := cfg.runCleanupFunc(ctx); err != nil {
			writeError("Run cleanup failed: " + err.Error())
			return statusError
		}
	}
	return statusSuccess
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
