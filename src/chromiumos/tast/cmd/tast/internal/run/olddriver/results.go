// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package olddriver

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/reporting"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/timing"
)

// These paths are relative to Config.ResDir.
const (
	systemLogsDir  = "system_logs" // dir containing DUT's system logs
	crashesDir     = "crashes"     // dir containing DUT's crashes
	testLogsDir    = "tests"       // dir containing dirs with details about individual tests
	fixtureLogsDir = "fixtures"    // dir containins dirs with details about individual fixtures

	testLogFilename         = "log.txt"      // file in testLogsDir/<test> containing test-specific log messages
	testOutputTimeFmt       = "15:04:05.000" // format for timestamps attached to test output
	testOutputFileRenameExt = ".from_test"   // extension appended to test output files conflicting with existing files

	defaultMsgTimeout = time.Minute           // default timeout for reading next control message
	incompleteTestMsg = "Test did not finish" // error message for incomplete tests
	noRunEndMsg       = "no RunEnd message"   // error message for missing RunEnd message
)

// entityState keeps track of states of a currently running entity.
type entityState struct {
	// result is the entity result message. It is modified as we receive control
	// messages.
	result resultsjson.Result

	// logFile is a file handle of the log file for the entity.
	logFile *os.File

	// logger is the SinkLogger that writes to logFile.
	logger *logging.SinkLogger

	// reportLogger is the SinkLogger that writes to logReportWriter.
	reportLogger *logging.SinkLogger

	// IntermediateOutDir is a directory path on the target where intermediate
	// output files for the test is saved.
	IntermediateOutDir string

	// FinalOutDir is a directory path on the host where final output files
	// for the test is saved.
	FinalOutDir string
}

// ErrTerminate is used when Tast shoulbe be terminated due to various reasons such as the case
// that maximum number of failures has been reached.
// This error should be wrapped with a different error to indicate the exact cause of termination.
// Callers should kill a running process on getting this error
var ErrTerminate = errors.New("testing jobs will be terminated")

var errUserReqTermination = errors.Wrap(ErrTerminate, "user requested tast to terminate testing")

// WriteResults writes results (including errors) to a JSON file in the results directory.
// It additionally logs each test's status via ctx.
// The cfg arg should be the same one that was passed to Run earlier.
// The complete arg should be true if the run finished successfully (regardless of whether
// any tests failed) and false if it was aborted.
// If cfg.CollectSysInfo is true, system information generated on the DUT during testing
// (e.g. logs and crashes) will also be written to the results dir.
func WriteResults(ctx context.Context, cfg *config.Config, state *config.State, results []*resultsjson.Result, initialSysInfo *protocol.SysInfoState, complete bool, drv *driver.Driver) error {
	// We don't want to bail out before writing test results if sys info collection fails,
	// but we'll still return the error later.
	sysInfoErr := drv.CollectSysInfo(ctx, initialSysInfo)
	if sysInfoErr != nil {
		logging.Info(ctx, "Failed collecting system info: ", sysInfoErr)
	}

	if err := reporting.WriteLegacyResults(filepath.Join(cfg.ResDir(), reporting.LegacyResultsFilename), results); err != nil {
		return err
	}

	if err := reporting.WriteJUnitXMLResults(filepath.Join(cfg.ResDir(), reporting.JUnitXMLFilename), results); err != nil {
		return err
	}

	reporting.WriteResultsToLogs(ctx, results, cfg.ResDir(), complete)

	return sysInfoErr
}

// copyAndRemoveFunc copies src on a DUT to dst on the local machine and then
// removes the directory on the DUT.
type copyAndRemoveFunc func(src, dst string) error

// diagnoseRunErrorFunc is called after a run error is encountered while reading test results to get additional
// information about the cause of the error. An empty string should be returned if additional information
// is unavailable.
// outDir is a path to a directory where extra output files can be written.
type diagnoseRunErrorFunc func(ctx context.Context, outDir string) string

// resultsHandler processes the output from a test binary.
type resultsHandler struct {
	cfg     *config.Config
	state   *config.State
	loggers *logging.MultiLogger

	runStart, runEnd time.Time                 // test-runner-reported times at which run started and ended
	numTests         int                       // total number of tests that are expected to run
	testsToRun       []string                  // names of tests that will be run in their expected order
	results          []*resultsjson.Result     // information about tests seen so far; the last element can be ongoing and shared with current
	currents         map[string]*entityState   // currently-running entities, if any
	seenTimes        map[string]int            // count of entity names seen so far
	stage            *timing.Stage             // current test's timing stage
	crf              copyAndRemoveFunc         // function used to copy and remove files from DUT
	diagFunc         diagnoseRunErrorFunc      // called to diagnose run errors; may be nil
	streamWriter     *reporting.StreamedWriter // used to write results as control messages are read
	pullers          sync.WaitGroup            // used to wait for puller goroutines
	terminated       bool                      // testing will be terminated.
}

func newResultsHandler(cfg *config.Config, state *config.State, crf copyAndRemoveFunc, df diagnoseRunErrorFunc) (*resultsHandler, error) {
	r := &resultsHandler{
		cfg:       cfg,
		state:     state,
		loggers:   logging.NewMultiLogger(),
		currents:  make(map[string]*entityState),
		seenTimes: make(map[string]int),
		crf:       crf,
		diagFunc:  df,
	}

	var err error
	if err = os.MkdirAll(r.cfg.ResDir(), 0755); err != nil {
		return nil, err
	}
	fn := filepath.Join(r.cfg.ResDir(), reporting.StreamedResultsFilename)
	if r.streamWriter, err = reporting.NewStreamedWriter(fn); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *resultsHandler) close() {
	for _, state := range r.currents {
		r.loggers.RemoveLogger(state.logger)
		state.logFile.Close()
	}
	r.currents = nil
	r.streamWriter.Close()
}

// handleRunStart handles RunStart control messages from test runners.
func (r *resultsHandler) handleRunStart(ctx context.Context, msg *control.RunStart) error {
	if !r.runStart.IsZero() {
		return errors.New("multiple RunStart messages")
	}
	r.runStart = msg.Time
	r.testsToRun = msg.TestNames
	if len(msg.TestNames) > 0 {
		r.numTests = len(msg.TestNames)
	} else {
		// Fallback path for old runners that don't set TestNames: https://crbug.com/889119
		r.numTests = msg.NumTests
	}
	return nil
}

// handleRunLog handles RunLog control messages from test runners.
func (r *resultsHandler) handleRunLog(ctx context.Context, msg *control.RunLog) error {
	logging.Infof(ctx, "[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleRunError handles RunError control messages from test runners.
func (r *resultsHandler) handleRunError(ctx context.Context, msg *control.RunError) error {
	// Just return an error to abort the run.
	return fmt.Errorf("%s:%d: %s",
		filepath.Base(msg.Error.File), msg.Error.Line, msg.Error.Reason)
}

// handleRunEnd handles RunEnd control messages from test runners.
func (r *resultsHandler) handleRunEnd(ctx context.Context, msg *control.RunEnd) error {
	if r.runStart.IsZero() {
		return errors.New("no RunStart message before RunEnd")
	}
	if len(r.currents) > 0 {
		return fmt.Errorf("got RunEnd message while %d entities still running", len(r.currents))
	}
	if !r.runEnd.IsZero() {
		return errors.New("multiple RunEnd messages")
	}
	r.runEnd = msg.Time
	return nil
}

// handleTestStart handles EntityStart control messages from test runners.
func (r *resultsHandler) handleTestStart(ctx context.Context, msg *control.EntityStart) error {
	if r.runStart.IsZero() {
		return errors.New("no RunStart message before EntityStart")
	}

	// TODO(crbug.com/1127169): Support timing log for fixtures.
	if msg.Info.Type == jsonprotocol.EntityTest {
		ctx, r.stage = timing.Start(ctx, msg.Info.Name)
	}

	relDir := testLogsDir
	if msg.Info.Type == jsonprotocol.EntityFixture {
		relDir = fixtureLogsDir
	}
	relFinalOutDir := filepath.Join(relDir, msg.Info.Name)

	// Add a number suffix to the output directory name in case of conflict.
	seenCnt := r.seenTimes[msg.Info.Name]
	if seenCnt > 0 {
		relFinalOutDir += fmt.Sprintf(".%d", seenCnt)
	}
	r.seenTimes[msg.Info.Name]++

	finalOutDir := filepath.Join(r.cfg.ResDir(), relFinalOutDir)
	state := &entityState{
		result: resultsjson.Result{
			Test:   *resultsjson.NewTestFromEntityInfo(&msg.Info),
			Start:  msg.Time,
			OutDir: finalOutDir,
		},
		IntermediateOutDir: msg.OutDir,
		FinalOutDir:        finalOutDir,
	}
	r.currents[msg.Info.Name] = state
	// Do not include fixture results in the output.
	// TODO(crbug.com/1135078): Consider reporting fixture results.
	if state.result.Type == jsonprotocol.EntityTest {
		r.results = append(r.results, &state.result)

		// Write a partial EntityResult object to record that we started the test.
		if err := r.streamWriter.Write(&state.result, false); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(state.result.OutDir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(state.result.OutDir, testLogFilename))
	if err != nil {
		return err
	}
	state.logFile = f
	state.logger = logging.NewSinkLogger(logging.LevelDebug, true, logging.NewWriterSink(state.logFile))
	r.loggers.AddLogger(state.logger)

	reportWriter := r.state.ReportClient.NewTestLogWriter(msg.Info.Name, filepath.Join(relFinalOutDir, testLogFilename))
	state.reportLogger = logging.NewSinkLogger(logging.LevelDebug, true, logging.NewWriterSink(reportWriter))
	r.loggers.AddLogger(state.reportLogger)

	logging.Infof(ctx, "Started %v %s", state.result.Type, state.result.Name)
	return nil
}

// handleTestLog handles EntityLog control messages from test runners.
func (r *resultsHandler) handleTestLog(ctx context.Context, msg *control.EntityLog) error {
	logging.Infof(ctx, "[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleTestError handles TestError control messages from test runners.
func (r *resultsHandler) handleTestError(ctx context.Context, msg *control.EntityError) error {
	state := r.currents[msg.Name]
	if state == nil {
		return fmt.Errorf("got TestError message for %s while it was not running", msg.Name)
	}

	state.result.Errors = append(state.result.Errors, resultsjson.Error{
		Time:   msg.Time,
		Reason: msg.Error.Reason,
		File:   msg.Error.File,
		Line:   msg.Error.Line,
		Stack:  msg.Error.Stack,
	})

	ts := msg.Time.Format(testOutputTimeFmt)
	logging.Infof(ctx, "[%s] Error at %s:%d: %s", ts, filepath.Base(msg.Error.File), msg.Error.Line, msg.Error.Reason)
	if msg.Error.Stack != "" {
		logging.Infof(ctx, "[%s] Stack trace:\n%s", ts, msg.Error.Stack)
	}
	return nil
}

func (r *resultsHandler) reportResult(ctx context.Context, res *resultsjson.Result) error {
	err := r.state.ReportClient.ReportResult(ctx, res)
	if err == reporting.ErrTerminate {
		r.terminated = true
		err = nil
	}
	return err
}

func (r *resultsHandler) handleTestEnd(ctx context.Context, msg *control.EntityEnd) error {
	state := r.currents[msg.Name]
	if state == nil {
		return fmt.Errorf("got TestEnd message for %s while it was not running", msg.Name)
	}

	// If the test reported timing stages, import them under the current stage.
	if r.stage != nil {
		if msg.TimingLog != nil {
			if err := r.stage.Import(msg.TimingLog); err != nil {
				logging.Infof(ctx, "Failed importing timing log for %v: %v", msg.Name, err)
			}
		}
		r.stage.End()
	}

	if len(msg.DeprecatedMissingSoftwareDeps) == 0 && len(msg.SkipReasons) == 0 {
		logging.Infof(ctx, "Completed %v %s in %v with %d error(s)",
			state.result.Type, msg.Name, msg.Time.Sub(state.result.Start).Round(time.Millisecond), len(state.result.Errors))
	} else {
		var reasons []string
		if len(msg.DeprecatedMissingSoftwareDeps) > 0 {
			reasons = append(reasons, "missing SoftwareDeps: "+strings.Join(msg.DeprecatedMissingSoftwareDeps, " "))
		}
		if len(msg.SkipReasons) > 0 {
			reasons = append(reasons, msg.SkipReasons...)
		}
		state.result.SkipReason = strings.Join(reasons, ", ")
		logging.Infof(ctx, "Skipped test %s due to missing dependencies: %s", msg.Name, state.result.SkipReason)
	}

	state.result.End = msg.Time

	// Replace the earlier partial TestResult object with the now-complete version.
	if state.result.Type == jsonprotocol.EntityTest {
		if len(state.result.Errors) > 0 {
			r.state.FailuresCount++
		}
		if err := r.reportResult(ctx, &state.result); err != nil {
			return err
		}
		if err := r.streamWriter.Write(&state.result, true); err != nil {
			return err
		}
	}

	r.loggers.RemoveLogger(state.logger)
	if err := state.logFile.Close(); err != nil {
		logging.Info(ctx, err)
	}

	if state.reportLogger != nil {
		r.loggers.RemoveLogger(state.reportLogger)
		state.reportLogger = nil
	}

	// Pull finished test output files in a separate goroutine.
	if state.IntermediateOutDir != "" {
		r.pullers.Add(1)
		go func() {
			defer r.pullers.Done()
			if err := moveTestOutputData(r.crf, state.IntermediateOutDir, state.FinalOutDir); err != nil {
				// This may be written to a log of an irrelevant test.
				logging.Infof(ctx, "Failed to copy output data of %s: %v", state.result.Name, err)
			}
		}()
	}

	delete(r.currents, msg.Name)
	r.stage = nil
	return nil
}

// handleHeartbeat handles Heartbeat control messages from test executables.
func (r *resultsHandler) handleHeartbeat(ctx context.Context, msg *control.Heartbeat) error {
	return nil
}

// moveTestOutputData moves per-test output data using crf. dstDir is the path
// to the destination directory, typically ending with testName. dstDir should
// already exist.
//
// This function is not associated to resultsHandler because it runs on a
// separate goroutine that does not own resultsHandler and can be suffered from
// data races.
func moveTestOutputData(crf copyAndRemoveFunc, outDir, dstDir string) error {
	tmpDir, err := ioutil.TempDir(filepath.Dir(dstDir), "pulltmp.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "files")
	if err := crf(outDir, srcDir); err != nil {
		return err
	}

	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		src := filepath.Join(srcDir, fi.Name())
		dst := filepath.Join(dstDir, fi.Name())

		// Check that the destination file doesn't already exist.
		// This could happen if a test creates an output file named log.txt.
		if _, err := os.Stat(dst); err == nil {
			dst += testOutputFileRenameExt
		}

		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// handleMessage handles generic control messages from test runners.
func (r *resultsHandler) handleMessage(ctx context.Context, msg control.Msg) error {
	switch v := msg.(type) {
	case *control.RunStart:
		return r.handleRunStart(ctx, v)
	case *control.RunLog:
		return r.handleRunLog(ctx, v)
	case *control.RunError:
		return r.handleRunError(ctx, v)
	case *control.RunEnd:
		return r.handleRunEnd(ctx, v)
	case *control.EntityStart:
		return r.handleTestStart(ctx, v)
	case *control.EntityLog:
		return r.handleTestLog(ctx, v)
	case *control.EntityError:
		return r.handleTestError(ctx, v)
	case *control.EntityEnd:
		return r.handleTestEnd(ctx, v)
	case *control.Heartbeat:
		return r.handleHeartbeat(ctx, v)
	default:
		return errors.New("unknown message type")
	}
}

// processMessages processes control messages and errors supplied by mch and ech.
// It returns results from executed tests and the names of tests that should have been
// run but were not started (likely due to an error). unstarted is nil if the list of
// tests was unavailable; see readTestOutput.
func (r *resultsHandler) processMessages(ctx context.Context, mch <-chan control.Msg, ech <-chan error) (
	results []*resultsjson.Result, unstarted []string, err error) {
	ctx = logging.AttachLogger(ctx, r.loggers)

	// If a test is incomplete when we finish reading messages, rewrite its entry at the
	// end of the streamed results file to make sure that all of its errors are recorded.
	defer func() {
		for _, state := range r.currents {
			state.result.Errors = append(state.result.Errors, resultsjson.Error{
				Time:   time.Now(),
				Reason: incompleteTestMsg,
			})

			if state.result.Type == jsonprotocol.EntityTest {
				// Ensure an end time is set properly and isn't default time epoch
				state.result.End = time.Now()
				r.state.FailuresCount++
				r.streamWriter.Write(&state.result, true)
				r.reportResult(ctx, &state.result)
			}
		}
		// Return errUserReqTermination if we get a terminate response from result server in the loop above.
		if err == nil && r.terminated {
			err = errUserReqTermination
		}
	}()

	timeout := defaultMsgTimeout
	if r.cfg.MsgTimeout() > 0 {
		timeout = r.cfg.MsgTimeout()
	}

	runErr := func() error {
		for {
			select {
			case msg := <-mch:
				if msg == nil {
					// If the channel is closed, we'll read the zero value.
					return nil
				}
				if err := r.handleMessage(ctx, msg); err != nil {
					return err
				}
				if r.terminated {
					return errUserReqTermination
				}
				if r.cfg.MaxTestFailures() > 0 && r.state.FailuresCount >= r.cfg.MaxTestFailures() {
					return errors.Wrapf(ErrTerminate, "the maximum number of test failures (%v) reached", r.cfg.MaxTestFailures())
				}
			case err := <-ech:
				return err
			case <-time.After(timeout):
				return fmt.Errorf("timed out after waiting %v for next message (probably lost SSH connection to DUT)",
					timeout)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}()

	// Wait for output file pullers to finish to avoid possible races between
	// pullers and diagnoseRunErrorFunc.
	r.pullers.Wait()

	if len(r.testsToRun) > 0 {
		// Let callers distinguish between an empty list and a missing list.
		unstarted = make([]string, 0)
		for _, name := range r.testsToRun {
			if r.seenTimes[name] == 0 {
				unstarted = append(unstarted, name)
			}
		}
	}

	if runErr == nil && r.runEnd.IsZero() {
		runErr = errors.New(noRunEndMsg)
	}
	if runErr != nil {
		// Try to get a more-specific diagnosis of what went wrong.
		msg := fmt.Sprintf("Got global error: %v", runErr)

		// Find an entity that started most recently.
		var lastState *entityState
		for _, state := range r.currents {
			if lastState == nil || state.result.Start.After(lastState.result.Start) {
				lastState = state
			}
		}

		if r.diagFunc != nil {
			outDir := r.cfg.ResDir()
			if lastState != nil {
				outDir = lastState.FinalOutDir
			}
			if dm := r.diagFunc(ctx, outDir); dm != "" {
				msg = dm
			}
		}

		// Log the message. If the error interrupted a test, the message will be written to the test's
		// log, and we also save it as an error within the test's result.
		logging.Info(ctx, msg)
		if lastState != nil {
			lastState.result.Errors = append(lastState.result.Errors, resultsjson.Error{
				Time:   time.Now(),
				Reason: msg,
			})
		}
		return r.results, unstarted, runErr
	}

	if len(unstarted) > 0 {
		return r.results, unstarted, fmt.Errorf("%v test(s) are unstarted", len(unstarted))
	}

	return r.results, unstarted, nil
}

// readMessages reads serialized control messages from r and passes them
// via mch. If an error is encountered, it is passed via ech and no more
// reads are performed. Channels are closed before returning.
func readMessages(r io.Reader, mch chan<- control.Msg, ech chan<- error) {
	mr := control.NewMessageReader(r)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			ech <- err
			break
		}
		mch <- msg
	}
	close(mch)
	close(ech)
}

// readTestOutput reads test runner output from r and returns the results, which should be passed to WriteResults.
//
// unstarted contains the names of tests that should have been run but were not started (likely due to an error).
// A zero-length slice indicates certainty that there are no more tests to run.
// A nil slice indicates that the list of tests to run was unavailable.
//
// df may be nil if diagnosis is unavailable.
func readTestOutput(ctx context.Context, cfg *config.Config, state *config.State, r io.Reader, crf copyAndRemoveFunc, df diagnoseRunErrorFunc) (
	results []*resultsjson.Result, unstarted []string, err error) {
	rh, err := newResultsHandler(cfg, state, crf, df)
	if err != nil {
		return nil, nil, err
	}
	defer rh.close()

	mch := make(chan control.Msg)
	ech := make(chan error)
	go readMessages(r, mch, ech)

	return rh.processMessages(ctx, mch, ech)
}