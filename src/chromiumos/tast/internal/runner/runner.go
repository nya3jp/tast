// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

const (
	statusSuccess    = 0 // runner was successful
	_                = 1 // deprecated
	statusBadArgs    = 2 // bad arguments were passed to the runner
	_                = 3 // deprecated
	_                = 4 // deprecated
	_                = 5 // deprecated
	statusTestFailed = 6 // one or more tests failed during manual run
	_                = 7 // deprecated
	_                = 8 // deprecated
)

// Run reads command-line flags from clArgs and performs the requested action.
// clArgs should typically be os.Args[1:]. The caller should exit with the
// returned status code.
func Run(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, scfg *StaticConfig) int {
	ctx := context.Background()

	if scfg.EnableSyslog {
		if l, err := logging.NewSyslogLogger(); err == nil {
			defer l.Close()
			ctx = logging.AttachLogger(ctx, l)
		}
	}
	logging.Debug(ctx, "Tast local_runner starts")
	defer logging.Debug(ctx, "Tast local_runner ends")

	// TODO(b/189332919): Remove this hack and write stack traces to stderr
	// once we finish migrating to gRPC-based protocol. This hack is needed
	// because JSON-based protocol is designed to write messages to stderr
	// in case of errors and thus Tast CLI consumes stderr.
	if os.Getenv("TAST_B189332919_STACK_TRACE_FD") == "3" {
		command.InstallSignalHandler(os.NewFile(3, ""), func(os.Signal) {})
	}

	args, err := parseArgs(clArgs, stderr, scfg)
	if err != nil {
		return command.WriteError(stderr, err)
	}

	switch args.Mode {
	case modeDeprecatedDirectRun:
		if err := deprecatedDirectRun(ctx, &args.DeprecatedDirectRunConfig, scfg, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	case modeRPC:
		if err := runRPCServer(scfg, stdin, stdout); err != nil {
			return command.WriteError(stderr, err)
		}
		return statusSuccess
	default:
		return command.WriteError(stderr, command.NewStatusErrorf(statusBadArgs, "invalid mode %v", args.Mode))
	}
}

func deprecatedDirectRun(ctx context.Context, drcfg *DeprecatedDirectRunConfig, scfg *StaticConfig, stdout io.Writer) error {
	lg := log.New(stdout, "", log.LstdFlags)

	matcher, err := testing.NewMatcher(drcfg.Patterns)
	if err != nil {
		return err
	}

	compat, err := startCompatServer(ctx, scfg, &protocol.HandshakeRequest{
		RunnerInitParams: &protocol.RunnerInitParams{
			BundleGlob: drcfg.BundleGlob,
		},
		BundleInitParams: &protocol.BundleInitParams{},
	})
	if err != nil {
		return err
	}
	defer compat.Close()

	cl := compat.Client()

	// Enumerate tests to run.
	res, err := cl.ListEntities(ctx, &protocol.ListEntitiesRequest{Features: drcfg.RunConfig(nil).GetFeatures()})
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

	// We expect to not match any tests if both local and remote tests are being run but the
	// user specified a pattern that matched only local or only remote tests rather than tests
	// of both types. Don't bother creating an out dir in that case.
	if len(testNames) == 0 {
		return errors.New("no tests matched")
	}

	rcfg := drcfg.RunConfig(testNames)

	created, err := setUpBaseOutDir(rcfg)
	if err != nil {
		return errors.Wrap(err, "failed to set up base out dir")
	}
	// If the runner was executed manually and an out dir wasn't specified, clean up the temp dir that was created.
	if created {
		defer os.RemoveAll(rcfg.GetDirs().GetOutDir())
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

	numTests := 0
	testFailed := false              // true if error seen for current test
	var failedTests []string         // names of tests with errors
	var startTime, endTime time.Time // start of first test and end of last test

	// Keep reading responses and convert them to control messages.
	for {
		res, err := srv.Recv()
		if err == io.EOF {
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
		if err != nil {
			return err
		}

		switch res := res.GetType().(type) {
		case *protocol.RunTestsResponse_RunLog:
			lg.Print(res.RunLog.GetText())
		case *protocol.RunTestsResponse_EntityStart:
			lg.Print("Running ", res.EntityStart.GetEntity().GetName())
			testFailed = false
			if numTests == 0 {
				startTime = res.EntityStart.GetTime().AsTime()
			}
		case *protocol.RunTestsResponse_EntityLog:
			lg.Print(res.EntityLog.GetText())
		case *protocol.RunTestsResponse_EntityError:
			e := res.EntityError.GetError()
			lg.Printf("Error: [%s:%d] %v", filepath.Base(e.GetLocation().GetFile()), e.GetLocation().GetLine(), e.GetReason())
			testFailed = true
		case *protocol.RunTestsResponse_EntityEnd:
			reasons := res.EntityEnd.GetSkip().GetReasons()
			if len(reasons) > 0 {
				lg.Printf("Skipped %s for missing deps: %s", res.EntityEnd.GetEntityName(), strings.Join(reasons, ", "))
			} else {
				lg.Print("Finished ", res.EntityEnd.GetEntityName())
			}
			lg.Print(strings.Repeat("-", 80))
			if testFailed {
				failedTests = append(failedTests, res.EntityEnd.GetEntityName())
			}
			numTests++
			endTime = res.EntityEnd.GetTime().AsTime()
		}
	}
}

// setUpBaseOutDir creates and assigns a temporary directory if rcfg.Dirs.OutDir is empty.
// It also ensures that the dir is accessible to all users. The returned boolean created
// indicates whether a new directory was created.
func setUpBaseOutDir(rcfg *protocol.RunConfig) (created bool, err error) {
	defer func() {
		if err != nil && created {
			os.RemoveAll(rcfg.GetDirs().GetOutDir())
			created = false
		}
	}()

	if rcfg.GetDirs().GetOutDir() == "" {
		if rcfg.GetDirs().OutDir, err = ioutil.TempDir("", "tast_out."); err != nil {
			return false, err
		}
		created = true
	} else {
		if _, err := os.Stat(rcfg.GetDirs().GetOutDir()); os.IsNotExist(err) {
			if err := os.MkdirAll(rcfg.GetDirs().GetOutDir(), 0755); err != nil {
				return false, err
			}
			created = true
		} else if err != nil {
			return false, err
		}
	}

	// Make the directory traversable in case a test wants to write a file as another user.
	// (Note that we can't guarantee that all the parent directories are also accessible, though.)
	if err := os.Chmod(rcfg.GetDirs().GetOutDir(), 0755); err != nil {
		return created, err
	}
	return created, nil
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
