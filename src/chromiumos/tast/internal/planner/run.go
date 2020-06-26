// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

const (
	exitTimeout     = 30 * time.Second // extra time granted to test-related funcs to exit
	preTestTimeout  = 15 * time.Second // timeout for TestConfig.PreTestFunc
	postTestTimeout = 15 * time.Second // timeout for TestConfig.PostTestFunc
)

// Config contains details about how the planner should run tests.
type Config struct {
	// DataDir is the path to the base directory containing test data files.
	DataDir string
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string
	// Vars contains names and values of runtime variables used to pass out-of-band data to tests.
	Vars map[string]string
	// Features contains software/hardware features the DUT has.
	Features dep.Features
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string
	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	BuildArtifactsURL string
	// RemoteData contains information relevant to remote tests.
	// It is nil for local tests.
	RemoteData *testing.RemoteData
	// PreTestFunc is run before TestInstance.Func (and TestInstance.Pre.Prepare, when applicable) if non-nil.
	// The returned closure is executed after PostTestFunc if not nil.
	PreTestFunc func(context.Context, *testing.State) func(context.Context, *testing.State)
	// PostTestFunc is run after TestInstance.Func (and TestInstance.Pre.Cleanup, when applicable) if non-nil.
	PostTestFunc func(context.Context, *testing.State)
}

// RunTests runs a set of tests, writing outputs to out.
//
// RunTests is responsible for building an efficient plan to run the given tests.
// Therefore the order of tests in the argument is ignored; it just specifies
// a set of tests to run.
//
// RunTests runs tests on goroutines. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func RunTests(ctx context.Context, tests []*testing.TestInstance, out OutputStream, pcfg *Config) error {
	testing.SortTests(tests)

	checkResults := make([]*testing.ShouldRunResult, len(tests))
	// If a test should run, the element of this array at the index will have a pointer to the next test (except last one).
	// We pass this information to runTest later to ensure that we don't incorrectly fail to close a precondition
	// if the final test using precondition is skipped: https://crbug.com/950499.
	nextTests := make([]*testing.TestInstance, len(tests))
	var runTests []*testing.TestInstance
	lastIdx := -1
	for i, t := range tests {
		checkResults[i] = tests[i].ShouldRun(&pcfg.Features)
		if checkResults[i].OK() {
			runTests = append(runTests, t)
			if lastIdx >= 0 {
				nextTests[lastIdx] = t
			}
			lastIdx = i
		}
	}

	// Download external data files.
	cl := devserver.NewClient(ctx, pcfg.Devservers)
	extdata.Ensure(ctx, pcfg.DataDir, pcfg.BuildArtifactsURL, runTests, cl)

	for i, t := range tests {
		tout := NewTestOutputStream(out, t.TestInfo())
		if checkResult := checkResults[i]; checkResult.OK() {
			if err := runTest(ctx, t, nextTests[i], tout, pcfg); err != nil {
				return err
			}
		} else {
			reportSkippedTest(tout, checkResult)
		}
	}
	return nil
}

// runTest runs a single test, writing outputs messages to tout.
//
// runTest runs a test on a goroutine. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func runTest(ctx context.Context, t, next *testing.TestInstance, tout *TestOutputStream, pcfg *Config) error {
	// Attach a log that the test can use to report timing events.
	timingLog := timing.NewLog()
	ctx = timing.NewContext(ctx, timingLog)

	tout.Start()
	defer tout.End(nil, timingLog)

	var outDir string
	if pcfg.OutDir != "" { // often left blank for unit tests
		outDir = filepath.Join(pcfg.OutDir, t.Name)
	}
	tcfg := &testing.TestConfig{
		DataDir:      filepath.Join(pcfg.DataDir, testing.RelativeDataDir(t.Pkg)),
		OutDir:       outDir,
		Vars:         pcfg.Vars,
		CloudStorage: testing.NewCloudStorage(pcfg.Devservers),
		RemoteData:   pcfg.RemoteData,
	}
	root := testing.NewRootState(t, tout, tcfg)
	stages := buildStages(t, next, pcfg, tcfg)

	ok := runStages(ctx, root, stages)
	if !ok {
		// If runStages reported that the test didn't finish, print diagnostic messages.
		const msg = "Test did not return on timeout (see log for goroutine dump)"
		tout.Error(testing.NewError(nil, msg, msg, 0))
		dumpGoroutines(tout)
	}

	if !ok {
		return errors.New("test did not return on timeout")
	}
	return nil
}

// buildStages builds stages to run a test.
//
// The time allotted to the test is generally the sum of t.Timeout and t.ExitTimeout, but
// additional time may be allotted for preconditions and pre/post-test hooks.
func buildStages(t, next *testing.TestInstance, pcfg *Config, tcfg *testing.TestConfig) []stage {
	var stages []stage
	addStage := func(f stageFunc, ctxTimeout, runTimeout time.Duration) {
		stages = append(stages, stage{f, ctxTimeout, runTimeout})
	}

	var postTestHook func(ctx context.Context, s *testing.State)

	// First, perform setup and run the pre-test function.
	addStage(func(ctx context.Context, root *testing.RootState) {
		root.RunWithTestState(ctx, func(ctx context.Context, s *testing.State) {
			// The test bundle is responsible for ensuring t.Timeout is nonzero before calling Run,
			// but we call s.Fatal instead of panicking since it's arguably nicer to report individual
			// test failures instead of aborting the entire run.
			if t.Timeout <= 0 {
				s.Fatal("Invalid timeout ", t.Timeout)
			}

			if tcfg.OutDir != "" { // often left blank for unit tests
				if err := os.MkdirAll(tcfg.OutDir, 0755); err != nil {
					s.Fatal("Failed to create output dir: ", err)
				}
				// Make the directory world-writable so that tests can create files as other users,
				// and set the sticky bit to prevent users from deleting other users' files.
				// (The mode passed to os.MkdirAll is modified by umask, so we need an explicit chmod.)
				if err := os.Chmod(tcfg.OutDir, 0777|os.ModeSticky); err != nil {
					s.Fatal("Failed to set permissions on output dir: ", err)
				}
			}

			// Make sure all required data files exist.
			for _, fn := range t.Data {
				fp := s.DataPath(fn)
				if _, err := os.Stat(fp); err == nil {
					continue
				}
				ep := fp + testing.ExternalErrorSuffix
				if data, err := ioutil.ReadFile(ep); err == nil {
					s.Errorf("Required data file %s missing: %s", fn, string(data))
				} else {
					s.Errorf("Required data file %s missing", fn)
				}
			}
			if s.HasError() {
				return
			}

			// In remote tests, reconnect to the DUT if needed.
			if tcfg.RemoteData != nil {
				dt := s.DUT()
				if !dt.Connected(ctx) {
					s.Log("Reconnecting to DUT")
					if err := dt.Connect(ctx); err != nil {
						s.Fatal("Failed to reconnect to DUT: ", err)
					}
				}
			}

			if pcfg.PreTestFunc != nil {
				postTestHook = pcfg.PreTestFunc(ctx, s)
			}
		})
	}, preTestTimeout, preTestTimeout+exitTimeout)

	// Prepare the test's precondition (if any) if setup was successful.
	if t.Pre != nil {
		addStage(func(ctx context.Context, root *testing.RootState) {
			if root.HasError() {
				return
			}
			root.RunWithPreState(ctx, func(ctx context.Context, s *testing.PreState) {
				s.Logf("Preparing precondition %q", t.Pre)

				if t.PreCtx == nil {
					// Associate PreCtx with TestContext for the first test.
					t.PreCtx, t.PreCtxCancel = context.WithCancel(testing.NewContext(context.Background(), s))
				}

				if next != nil && next.Pre == t.Pre {
					next.PreCtx = t.PreCtx
					next.PreCtxCancel = t.PreCtxCancel
				}

				root.SetPreCtx(t.PreCtx)
				root.SetPreValue(t.Pre.Prepare(ctx, s))
			})
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Next, run the test function itself if no errors have been reported so far.
	addStage(func(ctx context.Context, root *testing.RootState) {
		if root.HasError() {
			return
		}
		root.RunWithTestState(ctx, t.Func)
	}, t.Timeout, t.Timeout+timeoutOrDefault(t.ExitTimeout, exitTimeout))

	// If this is the final test using this precondition, close it
	// (even if setup, t.Pre.Prepare, or t.Func failed).
	if t.Pre != nil && (next == nil || next.Pre != t.Pre) {
		addStage(func(ctx context.Context, root *testing.RootState) {
			root.RunWithPreState(ctx, func(ctx context.Context, s *testing.PreState) {
				s.Logf("Closing precondition %q", t.Pre.String())
				t.Pre.Close(ctx, s)
				if t.PreCtxCancel != nil {
					t.PreCtxCancel()
				}
			})
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Finally, run the post-test functions unconditionally.
	addStage(func(ctx context.Context, root *testing.RootState) {
		root.RunWithTestState(ctx, func(ctx context.Context, s *testing.State) {
			if pcfg.PostTestFunc != nil {
				pcfg.PostTestFunc(ctx, s)
			}

			if postTestHook != nil {
				postTestHook(ctx, s)
			}
		})
	}, postTestTimeout, postTestTimeout+exitTimeout)

	return stages
}

// timeoutOrDefault returns timeout if positive or def otherwise.
func timeoutOrDefault(timeout, def time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return def
}

// reportSkippedTest is called instead of runTest for a test that is skipped due to
// having unsatisfied dependencies.
func reportSkippedTest(tout *TestOutputStream, result *testing.ShouldRunResult) {
	tout.Start()
	for _, msg := range result.Errors {
		_, fn, ln, _ := runtime.Caller(0)
		tout.Error(&testing.Error{
			Reason: msg,
			File:   fn,
			Line:   ln,
		})
	}
	tout.End(result.SkipReasons, nil)
}

// dumpGoroutines dumps all goroutines to tout.
func dumpGoroutines(tout *TestOutputStream) {
	tout.Log("Dumping all goroutines")
	if err := func() error {
		p := pprof.Lookup("goroutine")
		if p == nil {
			return errors.New("goroutine pprof not found")
		}
		var buf bytes.Buffer
		if err := p.WriteTo(&buf, 2); err != nil {
			return err
		}
		sc := bufio.NewScanner(&buf)
		for sc.Scan() {
			tout.Log(sc.Text())
		}
		return sc.Err()
	}(); err != nil {
		tout.Error(&testing.Error{
			Reason: fmt.Sprintf("Failed to dump goroutines: %v", err),
		})
	}
}
