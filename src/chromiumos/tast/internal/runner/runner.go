// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testcontext"
)

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	statusNoTests      = 4 // pattern(s) passed to runner didn't match any tests
	statusBundleFailed = 5 // test bundle exited with nonzero status
	statusTestFailed   = 6 // one or more tests failed during manual run
	statusInterrupted  = 7 // read end of stdout was closed or SIGINT was received
	statusTerminated   = 8 // SIGTERM was received
)

// Run reads command-line flags from clArgs (in the case of a manual run) or a JSON-marshaled
// RunnerArgs struct from stdin (when run by the tast command) and performs the requested action.
// Default arguments may be passed via args, which is filled with the additional args that are read.
// clArgs should typically be os.Args[1:].
// The caller should exit with the returned status code.
func Run(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, args *jsonprotocol.RunnerArgs, cfg *Config) int {
	// TODO(derat|nya): Consider applying timeout.
	ctx := context.TODO()
	if err := readArgs(clArgs, stdin, stderr, args, cfg); err != nil {
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case jsonprotocol.RunnerGetSysInfoStateMode:
		if err := handleGetSysInfoState(ctx, cfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerCollectSysInfoMode:
		if err := handleCollectSysInfo(ctx, args, cfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerGetDUTInfoMode:
		if err := handleGetDUTInfo(args, cfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerListTestsMode:
		_, tests, err := getBundlesAndTests(args)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := json.NewEncoder(stdout).Encode(tests); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerListFixturesMode:
		fixts, err := listFixtures(args.ListFixtures.BundleGlob)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		res := &jsonprotocol.RunnerListFixturesResult{Fixtures: fixts}
		if err := json.NewEncoder(stdout).Encode(res); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerRunTestsMode:
		if args.Report {
			// Success is always reported when running tests on behalf of the tast command.
			runTestsAndReport(ctx, args, cfg, stdout)
		} else if err := runTestsAndLog(ctx, args, cfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerDownloadPrivateBundlesMode:
		if err := handleDownloadPrivateBundles(ctx, args, cfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// runTestsAndReport runs bundles serially to perform testing and writes control messages to stdout.
// Fatal errors are reported via RunError messages, while test errors are reported via EntityError messages.
func runTestsAndReport(ctx context.Context, args *jsonprotocol.RunnerArgs, cfg *Config, stdout io.Writer) {
	mw := control.NewMessageWriter(stdout)

	hbw := control.NewHeartbeatWriter(mw, args.RunTests.BundleArgs.HeartbeatInterval)
	defer hbw.Stop()

	bundles, tests, statusErr := getBundlesAndTests(args)
	if statusErr != nil {
		mw.WriteMessage(newRunErrorMessagef(statusErr.Status(), "Failed enumerating tests: %v", statusErr))
		return
	}

	bundleArgs, err := args.BundleArgs(jsonprotocol.BundleRunTestsMode)
	if err != nil {
		mw.WriteMessage(newRunErrorMessagef(statusBadArgs, "Failed constructing bundle args: %v", err))
		return
	}

	testNames := make([]string, len(tests))
	for i, t := range tests {
		testNames[i] = t.Name
	}
	mw.WriteMessage(&control.RunStart{Time: time.Now(), TestNames: testNames, NumTests: len(tests)})

	ctx = testcontext.WithLogger(ctx, func(msg string) {
		mw.WriteMessage(&control.RunLog{Time: time.Now(), Text: msg})
	})

	if cfg.KillStaleRunners {
		killStaleRunners(ctx, syscall.SIGTERM)
	}

	// We expect to not match any tests if both local and remote tests are being run but the
	// user specified a pattern that matched only local or only remote tests rather than tests
	// of both types. Don't bother creating an out dir in that case.
	if len(tests) == 0 {
		if !args.Report {
			mw.WriteMessage(newRunErrorMessagef(statusNoTests, "No tests matched"))
			return
		}
	} else {
		created, err := setUpBaseOutDir(bundleArgs)
		if err != nil {
			mw.WriteMessage(newRunErrorMessagef(statusError, "Failed to set up base out dir: %v", err))
			return
		}
		// If the runner was executed manually and an out dir wasn't specified, clean up the temp dir that was created.
		if !args.Report && created {
			defer os.RemoveAll(bundleArgs.RunTests.OutDir)
		}

		// Hereafter, heartbeat messages are sent by bundles.
		hbw.Stop()

		for _, bundle := range bundles {
			// Copy each bundle's output (consisting of control messages) directly to stdout.
			if err := runBundle(bundle, bundleArgs, stdout); err != nil {
				// TODO(derat): The tast command currently aborts the run as soon as it sees a RunError
				// message, but consider changing that and continuing to run other bundles here.
				// If we execute additional bundles, be sure to return immediately for statusInterrupted.
				mw.WriteMessage(newRunErrorMessagef(err.Status(), "Bundle %v failed: %v", bundle, err))
				return
			}
		}
	}

	mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: bundleArgs.RunTests.OutDir})
}

// runTestsAndLog runs bundles serially to perform testing and logs human-readable results to stdout.
// Errors are returned both for fatal errors and for errors in individual tests.
func runTestsAndLog(ctx context.Context, args *jsonprotocol.RunnerArgs, cfg *Config, stdout io.Writer) *command.StatusError {
	lg := log.New(stdout, "", log.LstdFlags)

	pr, pw := io.Pipe()
	ch := make(chan *command.StatusError, 1)
	go func() { ch <- logMessages(pr, lg) }()

	runTestsAndReport(ctx, args, cfg, pw)
	pw.Close()
	return <-ch
}

// newRunErrorMessagef returns a new RunError control message.
func newRunErrorMessagef(status int, format string, args ...interface{}) *control.RunError {
	_, fn, ln, _ := runtime.Caller(1)
	return &control.RunError{
		Time: time.Now(),
		Error: jsonprotocol.Error{
			Reason: fmt.Sprintf(format, args...),
			File:   fn,
			Line:   ln,
			Stack:  string(debug.Stack()),
		},
		Status: status,
	}
}

// setUpBaseOutDir creates and assigns a temporary directory if args.RunTests.OutDir is empty.
// It also ensures that the dir is accessible to all users. The returned boolean created
// indicates whether a new directory was created.
func setUpBaseOutDir(args *jsonprotocol.BundleArgs) (created bool, err error) {
	defer func() {
		if err != nil && created {
			os.RemoveAll(args.RunTests.OutDir)
			created = false
		}
	}()

	// TODO(crbug.com/1000549): Stop handling empty OutDir here.
	// Latest tast command always sets OutDir. The only cases where OutDir is unset are
	// (1) the runner is run manually or (2) the runner is run by an old tast command.
	// Once old tast commands disappear, we can handle the manual run case specially,
	// then we can avoid setting OutDir here in the middle of the runner execution.
	if args.RunTests.OutDir == "" {
		if args.RunTests.OutDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return false, err
		}
		created = true
	} else {
		if _, err := os.Stat(args.RunTests.OutDir); os.IsNotExist(err) {
			if err := os.MkdirAll(args.RunTests.OutDir, 0755); err != nil {
				return false, err
			}
			created = true
		} else if err != nil {
			return false, err
		}
	}

	// Make the directory traversable in case a test wants to write a file as another user.
	// (Note that we can't guarantee that all the parent directories are also accessible, though.)
	if err := os.Chmod(args.RunTests.OutDir, 0755); err != nil {
		return created, err
	}
	return created, nil
}

// logMessages reads control messages from r and logs them to lg.
// It is used to print human-readable test output when the runner is executed manually rather
// than via the tast command. An error is returned if any EntityError messages are read.
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
		case *control.EntityStart:
			lg.Print("Running ", v.Info.Name)
			testFailed = false
			if numTests == 0 {
				startTime = v.Time
			}
		case *control.EntityLog:
			lg.Print(v.Text)
		case *control.EntityError:
			lg.Printf("Error: [%s:%d] %v", filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
			testFailed = true
		case *control.EntityEnd:
			var reasons []string
			if len(v.DeprecatedMissingSoftwareDeps) > 0 {
				reasons = append(reasons, "missing SoftwareDeps: "+strings.Join(v.DeprecatedMissingSoftwareDeps, " "))
			}
			if len(v.SkipReasons) > 0 {
				reasons = append(reasons, v.SkipReasons...)
			}
			if len(reasons) > 0 {
				lg.Printf("Skipped %s for missing deps: %s", v.Name, strings.Join(reasons, ", "))
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

// killStaleRunners sends sig to the process groups of any other processes sharing
// the current process's executable. Status messages and errors are logged using lf.
func killStaleRunners(ctx context.Context, sig syscall.Signal) {
	ourPID := os.Getpid()
	ourExe, err := os.Executable()
	if err != nil {
		testcontext.Log(ctx, "Failed to look up current executable: ", err)
		return
	}

	procs, err := process.Processes()
	if err != nil {
		testcontext.Log(ctx, "Failed to list processes while looking for stale runners: ", err)
		return
	}
	for _, proc := range procs {
		if int(proc.Pid) == ourPID {
			continue
		}
		if exe, err := proc.Exe(); err != nil || exe != ourExe {
			continue
		}
		testcontext.Logf(ctx, "Sending signal %d to stale %v process group %d", sig, ourExe, proc.Pid)
		if err := syscall.Kill(int(-proc.Pid), sig); err != nil {
			testcontext.Logf(ctx, "Failed killing process group %d: %v", proc.Pid, err)
		}
	}
}
