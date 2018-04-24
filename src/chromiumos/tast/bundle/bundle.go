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

	// Number of characters in prefixes from the log package, e.g. "2017/08/17 09:29:54 ".
	logPrefixLen = 20
)

// writeError writes an error to stderr.
func writeError(msg string) {
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		msg += "\n"
	}
	io.WriteString(os.Stderr, msg)
}

// runConfig describes how runTests should run tests.
type runConfig struct {
	// args contains arguments passed to the bundle by the runner.
	args *Args
	// mw is used to send control messages to the controlling process.
	// It is initialized by readArgs and is nil if the -report flag was not passed.
	mw *control.MessageWriter
	// tests contains tests to run. It is initialized by readArgs.
	tests []*testing.Test
	// setupFunc is run before each test if non-nil.
	setupFunc func() error
	// defaultTestTimeout contains the default maximum time allotted to each test.
	// It is only used if testing.Test.Timeout is unset.
	defaultTestTimeout time.Duration
}

// runTests runs tests per cfg.

// If an error is encountered in the test harness (as opposed to in a test), it is returned
// immediately.
//
// If cfg.mw is nil (i.e. tests were executed manually rather than by the tast command),
// failure is reported if any tests failed. If cfg.mw is non-nil, success is reported even
// if tests fail, as the tast command knows how to interpret test results.
func runTests(ctx context.Context, cfg *runConfig) int {
	if len(cfg.tests) == 0 {
		writeError("No tests matched by pattern(s)")
		return statusNoTests
	}

	for _, t := range cfg.tests {
		// Make a copy of the test with the default timeout if none was specified.
		test := *t
		if test.Timeout == 0 {
			test.Timeout = cfg.defaultTestTimeout
		}

		cfg.mw.WriteMessage(&control.TestStart{Time: time.Now(), Test: test})

		outDir := filepath.Join(cfg.args.OutDir, test.Name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			writeError("Failed to create output dir: " + err.Error())
			return statusError
		}

		if cfg.setupFunc != nil {
			if err := cfg.setupFunc(); err != nil {
				writeError("Failed to run setup: " + err.Error())
				return statusError
			}
		}
		ch := make(chan testing.Output)
		s := testing.NewState(ctx, ch, filepath.Join(cfg.args.DataDir, test.DataDir()), outDir,
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
