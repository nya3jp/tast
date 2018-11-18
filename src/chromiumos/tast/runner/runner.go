// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/command"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	statusNoTests      = 4 // pattern(s) passed to runner didn't match any tests
	statusBundleFailed = 5 // test bundle exited with nonzero status
	statusTestFailed   = 6 // one or more tests failed during manual run
)

// Run reads command-line flags from clArgs (in the case of a manual run) or a JSON-marshaled
// Args struct from stdin (when run by the tast command) and performs the requested action.
// Default arguments may be passed via args, which is filled with the additional args that are read.
// The caller should exit with the returned status code.
func Run(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, args *Args, rt RunnerType) int {
	if err := readArgs(clArgs, stdin, stderr, args, rt); err != nil {
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case GetSysInfoStateMode:
		if err := handleGetSysInfoState(args, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case CollectSysInfoMode:
		if err := handleCollectSysInfo(args, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case GetSoftwareFeaturesMode:
		if err := handleGetSoftwareFeatures(args, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case ListTestsMode:
		_, tests, err := getBundlesAndTests(args)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := testing.WriteTestsAsJSON(stdout, tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case RunTestsMode:
		if args.report {
			// Success is always reported when running tests on behalf of the tast command.
			runTestsAndReport(args, stdout)
		} else if err := runTestsAndLog(args, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// runTestsAndReport runs bundles serially to perform testing and writes control messages to stdout.
// Fatal errors are reported via RunError messages, while test errors are reported via TestError messages.
func runTestsAndReport(args *Args, stdout io.Writer) {
	mw := control.NewMessageWriter(stdout)
	bundles, tests, err := getBundlesAndTests(args)
	if err != nil {
		mw.WriteMessage(newRunErrorMessagef(err.Status(), "Failed enumerating tests: %v", err))
		return
	}

	mw.WriteMessage(&control.RunStart{Time: time.Now(), NumTests: len(tests)})

	bundleArgs := args.bundleArgs
	bundleArgs.Mode = bundle.RunTestsMode

	// We expect to not match any tests if both local and remote tests are being run but the
	// user specified a pattern that matched only local or only remote tests rather than tests
	// of both types. Don't bother creating an out dir in that case.
	if len(tests) == 0 {
		if !args.report {
			mw.WriteMessage(newRunErrorMessagef(statusNoTests, "No tests matched"))
			return
		}
	} else {
		created, err := setUpBaseOutDir(&bundleArgs)
		if err != nil {
			mw.WriteMessage(newRunErrorMessagef(statusError, "Failed to set up base out dir: %v", err))
			return
		}
		// If the runner was executed manually and an out dir wasn't specified, clean up the temp dir that was created.
		if !args.report && created {
			defer os.RemoveAll(bundleArgs.OutDir)
		}

		for _, bundle := range bundles {
			// Copy each bundle's output (consisting of control messages) directly to stdout.
			if err := runBundle(bundle, &bundleArgs, stdout); err != nil {
				// TODO(derat): The tast command currently aborts the run as soon as it sees a RunError
				// message, but consider changing that and continuing to run other bundles here.
				mw.WriteMessage(newRunErrorMessagef(err.Status(), "Bundle %v failed: %v", bundle, err))
				return
			}
		}
	}

	mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: bundleArgs.OutDir})
}

// runTestsAndLog runs bundles serially to perform testing and logs human-readable results to stdout.
// Errors are returned both for fatal errors and for errors in individual tests.
func runTestsAndLog(args *Args, stdout io.Writer) *command.StatusError {
	lg := log.New(stdout, "", log.LstdFlags)

	pr, pw := io.Pipe()
	ch := make(chan *command.StatusError, 1)
	go func() { ch <- logMessages(pr, lg) }()

	runTestsAndReport(args, pw)
	pw.Close()
	return <-ch
}

// newRunErrorMessagef returns a new RunError control message.
func newRunErrorMessagef(status int, format string, args ...interface{}) *control.RunError {
	_, fn, ln, _ := runtime.Caller(1)
	return &control.RunError{
		Time: time.Now(),
		Error: testing.Error{
			Reason: fmt.Sprintf(format, args...),
			File:   fn,
			Line:   ln,
			Stack:  string(debug.Stack()),
		},
		Status: status,
	}
}

// setUpBaseOutDir creates and assigns a temporary directory if args.OutDir is empty.
// It also ensures that the dir is accessible to all users.
func setUpBaseOutDir(args *bundle.Args) (created bool, err error) {
	if args.OutDir == "" {
		if args.OutDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return false, err
		}
		created = true
	}
	// Make the directory traversable in case a test wants to write a file as another user.
	// (Note that we can't guarantee that all the parent directories are also accessible, though.)
	if err = os.Chmod(args.OutDir, 0755); err != nil && created {
		os.RemoveAll(args.OutDir)
	}
	return created, err
}

// logMessages reads control messages from r and logs them to lg.
// It is used to print human-readable test output when the runner is executed manually rather
// than via the tast command. An error is returned if any TestError messages are read.
func logMessages(r io.Reader, lg *log.Logger) *command.StatusError {
	numTests := 0
	testFailed := false              // true if error seen for current test
	var failedTests []string         // names of tests with errors
	var startTime, endTime time.Time // start of first test and end of last test

	mr := control.NewMessageReader(r)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			return command.NewStatusErrorf(statusBundleFailed, "bundle produced bad output: %v", err)
		}
		switch v := msg.(type) {
		case *control.RunLog:
			lg.Print(v.Text)
		case *control.RunError:
			return command.NewStatusErrorf(v.Status, "error: [%s:%d] %v", filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
		case *control.TestStart:
			lg.Print("Running ", v.Test.Name)
			testFailed = false
			if numTests == 0 {
				startTime = v.Time
			}
		case *control.TestLog:
			lg.Print(v.Text)
		case *control.TestError:
			lg.Printf("Error: [%s:%d] %v", filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
			testFailed = true
		case *control.TestEnd:
			if len(v.MissingSoftwareDeps) > 0 {
				lg.Printf("Skipped %s for missing deps: %v", v.Name, v.MissingSoftwareDeps)
			} else {
				lg.Print("Finished ", v.Name)
			}
			lg.Print(strings.Repeat("-", 80))
			if testFailed {
				failedTests = append(failedTests, v.Name)
			}
			numTests++
			endTime = v.Time
		}
	}

	lg.Printf("Ran %d test(s) in %v", numTests, endTime.Sub(startTime).Round(time.Millisecond))
	if len(failedTests) > 0 {
		lg.Printf("%d failed:", len(failedTests))
		for _, t := range failedTests {
			lg.Print("  " + t)
		}
		return command.NewStatusErrorf(statusTestFailed, "test(s) failed")
	}
	return nil
}
