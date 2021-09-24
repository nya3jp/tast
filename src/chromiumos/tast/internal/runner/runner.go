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
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

const (
	statusSuccess      = 0 // runner was successful
	statusError        = 1 // unspecified error was encountered
	statusBadArgs      = 2 // bad arguments were passed to the runner
	statusNoBundles    = 3 // glob passed to runner didn't match any bundles
	_                  = 4 // deprecated
	statusBundleFailed = 5 // test bundle exited with nonzero status
	statusTestFailed   = 6 // one or more tests failed during manual run
	_                  = 7 // deprecated
	_                  = 8 // deprecated
)

// Run reads command-line flags from clArgs (in the case of a manual run) or a JSON-marshaled
// RunnerArgs struct from stdin (when run by the tast command) and performs the requested action.
// Default arguments may be passed via args, which is filled with the additional args that are read.
// clArgs should typically be os.Args[1:].
// The caller should exit with the returned status code.
func Run(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, args *jsonprotocol.RunnerArgs, scfg *StaticConfig) int {
	ctx := context.Background()

	if scfg.EnableSyslog {
		if l, err := logging.NewSyslogLogger(); err == nil {
			defer l.Close()
			ctx = logging.AttachLogger(ctx, l)
		}
	}

	// TODO(b/189332919): Remove this hack once we find the cause.
	if os.Getenv("TAST_B189332919_STACK_TRACE_FD") == "3" {
		command.InstallSignalHandler(os.NewFile(3, ""), func(os.Signal) {})
	}

	if err := readArgs(clArgs, stdin, stderr, args, scfg); err != nil {
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case jsonprotocol.RunnerGetSysInfoStateMode:
		if err := handleGetSysInfoState(ctx, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerCollectSysInfoMode:
		if err := handleCollectSysInfo(ctx, args, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerGetDUTInfoMode:
		if err := handleGetDUTInfo(args, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerListTestsMode:
		_, tests, err := getBundlesAndTests(args)
		if err != nil {
			return command.WriteError(stderr, err)
		}
		if err := WriteListTestsResultAsJSON(stdout, tests); err != nil {
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
			runTestsAndReport(ctx, args, scfg, stdout)
		} else if err := runTestsAndLog(ctx, args, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerDownloadPrivateBundlesMode:
		if err := handleDownloadPrivateBundles(ctx, args, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case jsonprotocol.RunnerRPCMode:
		if err := runRPCServer(scfg, stdin, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

// runTestsAndReport runs bundles serially to perform testing and writes control messages to stdout.
// Fatal errors are reported via RunError messages, while test errors are reported via EntityError messages.
func runTestsAndReport(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, stdout io.Writer) {
	mw := control.NewMessageWriter(stdout)

	hbw := control.NewHeartbeatWriter(mw, args.RunTests.BundleArgs.HeartbeatInterval)
	defer hbw.Stop()

	if err := runTestsCompat(ctx, mw, scfg, args); err != nil {
		mw.WriteMessage(newRunErrorMessagef(statusError, "%v", err))
	}
}

func runTestsCompat(ctx context.Context, mw *control.MessageWriter, scfg *StaticConfig, args *jsonprotocol.RunnerArgs) error {
	bundleArgs, err := args.BundleArgs(jsonprotocol.BundleRunTestsMode)
	if err != nil {
		return errors.Wrap(err, "failed constructing bundle args")
	}

	matcher, err := testing.NewMatcher(bundleArgs.RunTests.Patterns)
	if err != nil {
		return err
	}

	// Start an in-process gRPC server.
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	defer func() {
		cw.Close()
		cr.Close()
	}()
	go runRPCServer(scfg, sr, sw)

	rcfg, bcfg := bundleArgs.RunTests.Proto()

	params := &protocol.RunnerInitParams{
		BundleGlob: args.RunTests.BundleGlob,
	}
	conn, err := rpc.NewClient(ctx, cr, cw, &protocol.HandshakeRequest{
		RunnerInitParams: params,
		BundleInitParams: &protocol.BundleInitParams{
			Vars:         args.RunTests.BundleArgs.TestVars,
			BundleConfig: bcfg,
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to connect to in-process gRPC server")
	}
	defer conn.Close()

	cl := protocol.NewTestServiceClient(conn.Conn())

	// Enumerate tests to run.
	res, err := cl.ListEntities(ctx, &protocol.ListEntitiesRequest{Features: bundleArgs.RunTests.Features()})
	if err != nil {
		return errors.Wrap(err, "failed to enumerate entities in bundles")
	}

	var testNames []string
	for _, r := range res.Entities {
		e := r.GetEntity()
		if e.GetType() != protocol.EntityType_TEST {
			continue
		}
		if matcher.Match(e.GetName(), e.GetAttributes()) {
			testNames = append(testNames, e.GetName())
		}
	}
	sort.Strings(testNames)

	mw.WriteMessage(&control.RunStart{Time: time.Now(), TestNames: testNames, NumTests: len(testNames)})

	// We expect to not match any tests if both local and remote tests are being run but the
	// user specified a pattern that matched only local or only remote tests rather than tests
	// of both types. Don't bother creating an out dir in that case.
	if len(testNames) == 0 {
		if !args.Report {
			return errors.New("no tests matched")
		}
		mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: bundleArgs.RunTests.OutDir})
		return nil
	}

	created, err := setUpBaseOutDir(bundleArgs)
	if err != nil {
		return errors.Wrap(err, "failed to set up base out dir")
	}
	// If the runner was executed manually and an out dir wasn't specified, clean up the temp dir that was created.
	if !args.Report && created {
		defer os.RemoveAll(bundleArgs.RunTests.OutDir)
	}

	// Call RunTests method and send the initial request.
	srv, err := cl.RunTests(ctx)
	if err != nil {
		return errors.Wrap(err, "RunTests: failed to call")
	}

	initReq := &protocol.RunTestsRequest{Type: &protocol.RunTestsRequest_RunTestsInit{RunTestsInit: &protocol.RunTestsInit{RunConfig: rcfg}}}
	if err := srv.Send(initReq); err != nil {
		return errors.Wrap(err, "RunTests: failed to send initial request")
	}

	// Keep reading responses and convert them to control messages.
	for {
		res, err := srv.Recv()
		if err == io.EOF {
			mw.WriteMessage(&control.RunEnd{Time: time.Now(), OutDir: bundleArgs.RunTests.OutDir})
			return nil
		}
		if err != nil {
			return err
		}

		switch res := res.GetType().(type) {
		case *protocol.RunTestsResponse_RunLog:
			r := res.RunLog
			ts, err := ptypes.Timestamp(r.GetTime())
			if err != nil {
				return err
			}
			mw.WriteMessage(&control.RunLog{Time: ts, Text: r.GetText()})
		case *protocol.RunTestsResponse_EntityStart:
			r := res.EntityStart
			ts, err := ptypes.Timestamp(r.GetTime())
			if err != nil {
				return err
			}
			ei, err := jsonprotocol.EntityInfoFromProto(r.GetEntity())
			if err != nil {
				return err
			}
			mw.WriteMessage(&control.EntityStart{Time: ts, Info: *ei, OutDir: r.GetOutDir()})
		case *protocol.RunTestsResponse_EntityLog:
			r := res.EntityLog
			ts, err := ptypes.Timestamp(r.GetTime())
			if err != nil {
				return err
			}
			mw.WriteMessage(&control.EntityLog{Time: ts, Name: r.GetEntityName(), Text: r.GetText()})
		case *protocol.RunTestsResponse_EntityError:
			r := res.EntityError
			ts, err := ptypes.Timestamp(r.GetTime())
			if err != nil {
				return err
			}
			mw.WriteMessage(&control.EntityError{Time: ts, Name: r.GetEntityName(), Error: *jsonprotocol.ErrorFromProto(r.GetError())})
		case *protocol.RunTestsResponse_EntityEnd:
			r := res.EntityEnd
			ts, err := ptypes.Timestamp(r.GetTime())
			if err != nil {
				return err
			}
			timingLog, err := timing.LogFromProto(r.GetTimingLog())
			if err != nil {
				return err
			}
			mw.WriteMessage(&control.EntityEnd{Time: ts, Name: r.GetEntityName(), SkipReasons: r.GetSkip().GetReasons(), TimingLog: timingLog})
		}
	}
}

// runTestsAndLog runs bundles serially to perform testing and logs human-readable results to stdout.
// Errors are returned both for fatal errors and for errors in individual tests.
func runTestsAndLog(ctx context.Context, args *jsonprotocol.RunnerArgs, scfg *StaticConfig, stdout io.Writer) *command.StatusError {
	lg := log.New(stdout, "", log.LstdFlags)

	pr, pw := io.Pipe()
	ch := make(chan *command.StatusError, 1)
	go func() { ch <- logMessages(pr, lg) }()

	runTestsAndReport(ctx, args, scfg, pw)
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
func killStaleRunners(ctx context.Context, sig unix.Signal) {
	ourPID := os.Getpid()
	ourExe, err := os.Executable()
	if err != nil {
		logging.Info(ctx, "Failed to look up current executable: ", err)
		return
	}

	procs, err := process.Processes()
	if err != nil {
		logging.Info(ctx, "Failed to list processes while looking for stale runners: ", err)
		return
	}
	for _, proc := range procs {
		if int(proc.Pid) == ourPID {
			continue
		}
		if exe, err := proc.Exe(); err != nil || exe != ourExe {
			continue
		}
		logging.Infof(ctx, "Sending signal %d to stale %v process group %d", sig, ourExe, proc.Pid)
		if err := unix.Kill(int(-proc.Pid), sig); err != nil {
			logging.Infof(ctx, "Failed killing process group %d: %v", proc.Pid, err)
		}
	}
}

// WriteListTestsResultAsJSON packs given parameters as a ListTestsResult, and
// writes it to w in JSON format. This function is exported for unit tests to
// implement fake runner.
func WriteListTestsResultAsJSON(w io.Writer, tests []*jsonprotocol.EntityWithRunnabilityInfo) error {
	var r jsonprotocol.RunnerListTestsResult = tests
	return json.NewEncoder(w).Encode(r)
}
