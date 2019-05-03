// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/bundle"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

const (
	// These paths are relative to Config.ResDir.
	resultsFilename         = "results.json"           // file containing JSON array of TestResult objects
	streamedResultsFilename = "streamed_results.jsonl" // file containing stream of newline-separated JSON TestResult objects
	systemLogsDir           = "system_logs"            // dir containing DUT's system logs
	crashesDir              = "crashes"                // dir containing DUT's crashes
	testLogsDir             = "tests"                  // dir containing dirs with details about individual tests

	testLogFilename         = "log.txt"      // file in testLogsDir/<test> containing test-specific log messages
	testOutputTimeFmt       = "15:04:05.000" // format for timestamps attached to test output
	testOutputFileRenameExt = ".from_test"   // extension appended to test output files conflicting with existing files

	defaultMsgTimeout = time.Minute           // default timeout for reading next control message
	incompleteTestMsg = "Test did not finish" // error message for incomplete tests
)

// TestResult contains the results from a single test.
// Fields are exported so they can be marshaled by the json package.
type TestResult struct {
	// Test contains basic information about the test. This is not a runnable
	// testing.Test struct; only fields that can be marshaled to JSON are set.
	testing.Test
	// Errors contains errors encountered while running the test.
	// If it is empty, the test passed.
	Errors []TestError `json:"errors"`
	// Start is the time at which the test started (as reported by the test bundle).
	Start time.Time `json:"start"`
	// End is the time at which the test completed (as reported by the test bundle).
	// It may hold the zero value (0001-01-01T00:00:00Z) to indicate that the test did not complete
	// (typically indicating that the test bundle, test runner, or DUT crashed mid-test).
	// In this case, at least one error will also be present indicating that the test was incomplete.
	End time.Time `json:"end"`
	// OutDir is the directory into which test output is stored.
	OutDir string `json:"outDir"`
	// SkipReason contains a human-readable explanation of why the test was skipped.
	// It is empty if the test actually ran.
	SkipReason string `json:"skipReason"`

	testStartMsgTime time.Time // time at which TestStart control message was received
	logFile          *os.File  // test's log file
}

// TestError describes an error that occurred while running a test.
// Most of its fields are defined in the Error struct in chromiumos/tast/testing.
// This struct just adds an additional "Time" field.
type TestError struct {
	// Time contains the time at which the error occurred (as reported by the test bundle).
	Time time.Time `json:"time"`
	// Error is an embedded struct describing the error.
	testing.Error
}

// WriteResults writes results (including errors) to a JSON file in the results directory.
// It additionally logs each test's status to cfg.Logger.
// The complete arg should be true if the run finished successfully (regardless of whether
// any tests failed) and false if it was aborted.
// If cfg.CollectSysInfo is true, system information generated on the DUT during testing
// (e.g. logs and crashes) will also be written to the results dir.
func WriteResults(ctx context.Context, cfg *Config, results []TestResult, complete bool) error {
	f, err := os.Create(filepath.Join(cfg.ResDir, resultsFilename))
	if err != nil {
		return err
	}
	defer f.Close()

	// We don't want to bail out before writing test results if sys info collection fails,
	// but we'll still return the error later.
	sysInfoErr := collectSysInfo(ctx, cfg)
	if sysInfoErr != nil {
		cfg.Logger.Log("Failed collecting system info: ", sysInfoErr)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(results); err != nil {
		return err
	}

	ml := 0
	for _, res := range results {
		if len(res.Name) > ml {
			ml = len(res.Name)
		}
	}

	sep := strings.Repeat("-", 80)
	cfg.Logger.Log(sep)

	for _, res := range results {
		pn := fmt.Sprintf("%-"+strconv.Itoa(ml)+"s", res.Name)
		if len(res.Errors) == 0 {
			if res.SkipReason == "" {
				cfg.Logger.Log(pn + "  [ PASS ]")
			} else {
				cfg.Logger.Log(pn + "  [ SKIP ] " + res.SkipReason)
			}
		} else {
			const failStr = "  [ FAIL ] "
			for i, te := range res.Errors {
				if i == 0 {
					cfg.Logger.Log(pn + failStr + te.Reason)
				} else {
					cfg.Logger.Log(strings.Repeat(" ", ml+len(failStr)) + te.Reason)
				}
			}
		}
	}

	if complete {
		// Let the user know if one or more of the globs that they supplied didn't match any tests.
		if pats := unmatchedTestPatterns(cfg.Patterns, results); len(pats) > 0 {
			cfg.Logger.Log("")
			cfg.Logger.Log("One or more test patterns did not match any tests:")
			for _, p := range pats {
				cfg.Logger.Log("  " + p)
			}
		}
	} else {
		// If the run didn't finish, log an additional message after the individual results
		// to make it clearer that all is not well.
		cfg.Logger.Log("")
		cfg.Logger.Log("Run did not finish successfully; results are incomplete")
	}

	cfg.Logger.Log(sep)
	cfg.Logger.Log("Results saved to ", cfg.ResDir)
	return sysInfoErr
}

// copyAndRemoveFunc copies src on a DUT to dst on the local machine and then
// removes src from the DUT.
type copyAndRemoveFunc func(src, dst string) error

// diagnoseRunErrorFunc is called after a run error is encountered while reading test results to get additional
// information about the cause of the error. An empty string should be returned if additional information
// is unavailable.
type diagnoseRunErrorFunc func(ctx context.Context) string

// resultsHandler processes the output from a test binary.
// TODO(nya): Delete seenHeartbeat after 20190701.
type resultsHandler struct {
	cfg *Config

	runStart, runEnd time.Time              // test-runner-reported times at which run started and ended
	numTests         int                    // total number of tests that are expected to run
	results          []TestResult           // information about completed tests
	res              *TestResult            // currently-running test, if any (i.e. last element of results)
	testNames        map[string]struct{}    // names of tests seen so far
	stage            *timing.Stage          // current test's timing stage
	crf              copyAndRemoveFunc      // function used to copy and remove files from DUT
	diagFunc         diagnoseRunErrorFunc   // called to diagnose run errors; may be nil
	streamWriter     *streamedResultsWriter // used to write results as control messages are read
	seenHeartbeat    bool                   // whether heartbeat messages were ever seen
}

func newResultsHandler(cfg *Config, crf copyAndRemoveFunc, df diagnoseRunErrorFunc) (*resultsHandler, error) {
	r := &resultsHandler{
		cfg:       cfg,
		results:   make([]TestResult, 0),
		testNames: make(map[string]struct{}),
		crf:       crf,
		diagFunc:  df,
	}

	var err error
	if err = os.MkdirAll(r.cfg.ResDir, 0755); err != nil {
		return nil, err
	}
	fn := filepath.Join(r.cfg.ResDir, streamedResultsFilename)
	if r.streamWriter, err = newStreamedResultsWriter(fn); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *resultsHandler) close() {
	if r.res != nil {
		r.cfg.Logger.RemoveWriter(r.res.logFile)
		r.res.logFile.Close()
	}
	r.streamWriter.close()
}

// setProgress updates the currently displayed progress to display the number of completed vs.
// total tests and the message s.
func (r *resultsHandler) setProgress(s string) {
	if s != "" {
		s = " " + s
	}
	r.cfg.Logger.Status(fmt.Sprintf("[%d/%d]%s", len(r.results), r.numTests, s))
}

// handleRunStart handles RunStart control messages from test executables.
func (r *resultsHandler) handleRunStart(ctx context.Context, msg *control.RunStart) error {
	if !r.runStart.IsZero() {
		return errors.New("multiple RunStart messages")
	}
	r.runStart = msg.Time
	r.numTests = msg.NumTests
	r.setProgress("Starting testing")
	return nil
}

// handleRunLog handles RunLog control messages from test executables.
func (r *resultsHandler) handleRunLog(ctx context.Context, msg *control.RunLog) error {
	r.cfg.Logger.Logf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleRunError handles RunError control messages from test executables.
func (r *resultsHandler) handleRunError(ctx context.Context, msg *control.RunError) error {
	// Just return an error to abort the run.
	return fmt.Errorf("%s:%d: %s",
		filepath.Base(msg.Error.File), msg.Error.Line, msg.Error.Reason)
}

// handleRunEnd handles RunEnd control messages from test executables.
func (r *resultsHandler) handleRunEnd(ctx context.Context, msg *control.RunEnd) error {
	if r.runStart.IsZero() {
		return errors.New("no RunStart message before RunEnd")
	}
	if r.res != nil {
		return fmt.Errorf("got RunEnd message while %s still running", r.res.Name)
	}
	if !r.runEnd.IsZero() {
		return errors.New("multiple RunEnd messages")
	}
	r.runEnd = msg.Time

	if len(msg.OutDir) != 0 {
		r.setProgress("Copying output files")
		localOutDir := filepath.Join(r.cfg.ResDir, "out.tmp")
		if err := r.crf(msg.OutDir, localOutDir); err != nil {
			r.cfg.Logger.Log("Failed to copy test output data: ", err)
		} else if err := r.moveTestOutputData(localOutDir); err != nil {
			r.cfg.Logger.Log("Failed to move test output data: ", err)
		}
	}
	return nil
}

// handleTestStart handles TestStart control messages from test executables.
func (r *resultsHandler) handleTestStart(ctx context.Context, msg *control.TestStart) error {
	if r.runStart.IsZero() {
		return errors.New("no RunStart message before TestStart")
	}
	if r.res != nil {
		return fmt.Errorf("got TestStart message for %s while %s still running",
			msg.Test.Name, r.res.Name)
	}
	if _, ok := r.testNames[msg.Test.Name]; ok {
		return fmt.Errorf("got TestStart message for already-seen test %s -- two tests with same name?",
			msg.Test.Name)
	}
	r.stage = timing.Start(ctx, msg.Test.Name)

	r.results = append(r.results, TestResult{
		Test:             msg.Test,
		Start:            msg.Time,
		OutDir:           r.getTestOutputDir(msg.Test.Name),
		testStartMsgTime: time.Now(),
	})
	r.res = &r.results[len(r.results)-1]
	r.testNames[msg.Test.Name] = struct{}{}

	// Write a partial TestResult object to record that we started the test.
	var err error
	if err = r.streamWriter.write(r.res, false); err != nil {
		return err
	}

	if err = os.MkdirAll(r.res.OutDir, 0755); err != nil {
		return err
	}
	if r.res.logFile, err = os.Create(
		filepath.Join(r.res.OutDir, testLogFilename)); err != nil {
		return err
	}
	if err = r.cfg.Logger.AddWriter(r.res.logFile, log.LstdFlags); err != nil {
		return err
	}

	r.cfg.Logger.Log("Started test ", r.res.Name)
	r.setProgress("Running " + r.res.Name)
	return nil
}

// handleTestLog handles TestLog control messages from test executables.
func (r *resultsHandler) handleTestLog(ctx context.Context, msg *control.TestLog) error {
	r.cfg.Logger.Logf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleTestError handles TestError control messages from test executables.
func (r *resultsHandler) handleTestError(ctx context.Context, msg *control.TestError) error {
	if r.res == nil {
		return errors.New("got TestError message while no test was running")
	}

	r.res.Errors = append(r.res.Errors, TestError{msg.Time, msg.Error})

	ts := msg.Time.Format(testOutputTimeFmt)
	r.cfg.Logger.Logf("[%s] Error at %s:%d: %s", ts, filepath.Base(msg.Error.File), msg.Error.Line, msg.Error.Reason)
	if msg.Error.Stack != "" {
		r.cfg.Logger.Debugf("[%s] Stack trace:\n%s", ts, msg.Error.Stack)
	}
	return nil
}

// handleTestEnd handles TestEnd control messages from test executables.
func (r *resultsHandler) handleTestEnd(ctx context.Context, msg *control.TestEnd) error {
	if r.res == nil || msg.Name != r.res.Name {
		return fmt.Errorf("got TestEnd message for not-started test %s", msg.Name)
	}

	if r.stage != nil {
		// If the test reported timing stages, import them under the current stage.
		if tl, ok := timing.FromContext(ctx); ok && msg.TimingLog != nil && !msg.TimingLog.Empty() {
			if err := tl.Import(msg.TimingLog); err != nil {
				r.cfg.Logger.Logf("Failed importing timing log for %v: %v", msg.Name, err)
			}
		}
		r.stage.End()
	}

	if len(msg.MissingSoftwareDeps) == 0 {
		r.cfg.Logger.Logf("Completed test %s in %v with %d error(s)",
			msg.Name, msg.Time.Sub(r.res.Start).Round(time.Millisecond), len(r.res.Errors))
	} else {
		r.cfg.Logger.Logf("Skipped test %s due to missing dependencies: %s",
			msg.Name, strings.Join(msg.MissingSoftwareDeps, " "))
		r.res.SkipReason = "missing deps: " + strings.Join(msg.MissingSoftwareDeps, " ")
	}

	r.res.End = msg.Time

	// Replace the earlier partial TestResult object with the now-complete version.
	if err := r.streamWriter.write(r.res, true); err != nil {
		return err
	}

	if err := r.cfg.Logger.RemoveWriter(r.res.logFile); err != nil {
		r.cfg.Logger.Log(err)
	}
	if err := r.res.logFile.Close(); err != nil {
		r.cfg.Logger.Log(err)
	}
	r.res = nil
	r.stage = nil
	return nil
}

// handleHeartbeat handles Heartbeat control messages from test executables.
func (r *resultsHandler) handleHeartbeat(ctx context.Context, msg *control.Heartbeat) error {
	r.seenHeartbeat = true
	return nil
}

// getTestOutputDir returns the directory into which data should be stored for a test named testName.
func (r *resultsHandler) getTestOutputDir(testName string) string {
	return filepath.Join(r.cfg.ResDir, testLogsDir, testName)
}

// moveTestOutputData moves per-test output data from test-named directories under srcBase
// to the corresponding test directories under r.cfg.ResDir.
func (r *resultsHandler) moveTestOutputData(srcBase string) error {
	if _, err := os.Stat(srcBase); os.IsNotExist(err) {
		return nil
	}

	// Iterate over per-test directories created by tests.
	srcDirs, err := ioutil.ReadDir(srcBase)
	if err != nil {
		return err
	}
	for _, fi := range srcDirs {
		// We created a dest dir for each test when we saw its TestStart message.
		// If we see a src dir without a matching dest dir, something strange happened.
		dstDir := r.getTestOutputDir(fi.Name())
		if _, err = os.Stat(dstDir); os.IsNotExist(err) {
			r.cfg.Logger.Log("Skipping unexpected output dir ", fi.Name())
			continue
		}

		// Iterate over the files in each directory.
		srcDir := filepath.Join(srcBase, fi.Name())
		files, err := ioutil.ReadDir(srcDir)
		if err != nil {
			return err
		}
		for _, fi2 := range files {
			src := filepath.Join(srcDir, fi2.Name())
			dst := filepath.Join(dstDir, fi2.Name())

			// Check that the destination file doesn't already exist.
			// This could happen if a test creates an output file named log.txt.
			if _, err := os.Stat(dst); err == nil {
				dst += testOutputFileRenameExt
				r.cfg.Logger.Logf("File %v already exists; renaming test output to %v",
					filepath.Base(fi2.Name()), filepath.Base(dst))
			}

			if err = os.Rename(src, dst); err != nil {
				return err
			}
		}
	}

	if err = os.RemoveAll(srcBase); err != nil {
		r.cfg.Logger.Log("Failed to remove temp dir: ", err)
	}
	return nil
}

// nextMessageTimeout calculates the maximum amount of time to wait for the next
// control message from the test executable.
func (r *resultsHandler) nextMessageTimeout(now time.Time) time.Duration {
	timeout := defaultMsgTimeout
	if r.cfg.msgTimeout > 0 {
		timeout = r.cfg.msgTimeout
	}

	// If the bundle supports heartbeat messages, we don't need to consider
	// test timeouts.
	if r.seenHeartbeat {
		return timeout
	}

	// Otherwise, if we're in the middle of a test, add its timeout.
	if r.res != nil {
		elapsed := now.Sub(r.res.testStartMsgTime)
		if tm := r.res.Timeout + r.res.AdditionalTime; elapsed < tm {
			timeout += tm - elapsed
		}
	}
	return timeout
}

// handleMessage handles generic control messages from test executables.
func (r *resultsHandler) handleMessage(ctx context.Context, msg interface{}) error {
	switch v := msg.(type) {
	case *control.RunStart:
		return r.handleRunStart(ctx, v)
	case *control.RunLog:
		return r.handleRunLog(ctx, v)
	case *control.RunError:
		return r.handleRunError(ctx, v)
	case *control.RunEnd:
		return r.handleRunEnd(ctx, v)
	case *control.TestStart:
		return r.handleTestStart(ctx, v)
	case *control.TestLog:
		return r.handleTestLog(ctx, v)
	case *control.TestError:
		return r.handleTestError(ctx, v)
	case *control.TestEnd:
		return r.handleTestEnd(ctx, v)
	case *control.Heartbeat:
		return r.handleHeartbeat(ctx, v)
	default:
		return errors.New("unknown message type")
	}
}

// processMessages processes control messages and errors supplied by mch and ech.
func (r *resultsHandler) processMessages(ctx context.Context, mch <-chan interface{}, ech <-chan error) ([]TestResult, error) {
	// If a test is incomplete when we finish reading messages, rewrite its entry at the
	// end of the streamed results file to make sure that all of its errors are recorded.
	defer func() {
		if r.res != nil {
			r.res.Errors = append(r.res.Errors, TestError{time.Now(), testing.Error{Reason: incompleteTestMsg}})
			r.streamWriter.write(r.res, true)
		}
	}()

	if runErr := func() error {
		for {
			timeout := r.nextMessageTimeout(time.Now())
			select {
			case msg := <-mch:
				if msg == nil {
					// If the channel is closed, we'll read the zero value.
					return nil
				}
				if err := r.handleMessage(ctx, msg); err != nil {
					return err
				}
			case err := <-ech:
				return err
			case <-time.After(timeout):
				return fmt.Errorf("timed out after waiting %v for next message (probably lost SSH connection to DUT)",
					timeout.Round(time.Millisecond))
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}(); runErr != nil {
		// Try to get a more-specific diagnosis of what went wrong.
		msg := fmt.Sprintf("Got global error: %v", runErr)
		if r.diagFunc != nil {
			if dm := r.diagFunc(ctx); dm != "" {
				msg = dm
			}
		}

		// Log the message. If the error interrupted a test, the message will be written to the test's
		// log, and we also save it as an error within the test's result.
		r.cfg.Logger.Log(msg)
		if r.res != nil {
			r.res.Errors = append(r.res.Errors, TestError{time.Now(), testing.Error{Reason: msg}})
		}
		return r.results, runErr
	}

	if r.runEnd.IsZero() {
		return r.results, errors.New("no RunEnd message")
	}
	if len(r.results) != r.numTests {
		return r.results, fmt.Errorf("got results for %v test(s); expected %v", len(r.results), r.numTests)
	}

	return r.results, nil
}

// streamedResultsWriter is used by resultsHandler to write a stream of JSON-marshaled TestResults
// objects to a file.
type streamedResultsWriter struct {
	f          *os.File
	enc        *json.Encoder
	lastOffset int64 // file offset of the start of the last-written result
}

// newStreamedResultsWriter creates and returns a new streamedResultsWriter for writing to
// a file at path p. If the file already exists, new results are appended to it.
func newStreamedResultsWriter(p string) (*streamedResultsWriter, error) {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	eof, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &streamedResultsWriter{f: f, enc: json.NewEncoder(f), lastOffset: eof}, nil
}

func (w *streamedResultsWriter) close() {
	w.f.Close()
}

// write writes the JSON-marshaled representation of res to the file.
// If update is true, the previous result that was written by this instance is overwritten.
// Concurrent calls are not supported (note that tests are run serially, and runners send
// control messages to the tast process serially as well).
func (w *streamedResultsWriter) write(res *TestResult, update bool) error {
	var err error
	if update {
		// If we're replacing the last record, seek back to the beginning of it and leave the saved offset unmodified.
		if _, err = w.f.Seek(w.lastOffset, io.SeekStart); err != nil {
			return err
		}
		if err = w.f.Truncate(w.lastOffset); err != nil {
			return err
		}
	} else {
		// Otherwise, use Seek to record the current offset before we write.
		if w.lastOffset, err = w.f.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
	}

	return w.enc.Encode(res)
}

// readMessages reads serialized control messages from r and passes them
// via mch. If an error is encountered, it is passed via ech and no more
// reads are performed. Channels are closed before returning.
func readMessages(r io.Reader, mch chan<- interface{}, ech chan<- error) {
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

// readTestOutput reads test output from r and returns the results, which should be passed to WriteResults.
// df may be nil if diagnosis is unavailable.
func readTestOutput(ctx context.Context, cfg *Config, r io.Reader, crf copyAndRemoveFunc, df diagnoseRunErrorFunc) (
	[]TestResult, error) {
	rh, err := newResultsHandler(cfg, crf, df)
	if err != nil {
		return nil, err
	}
	defer rh.close()

	mch := make(chan interface{})
	ech := make(chan error)
	go readMessages(r, mch, ech)

	return rh.processMessages(ctx, mch, ech)
}

// readTestList decodes JSON-serialized testing.Test objects from r and
// copies them into an array of TestResult objects.
func readTestList(r io.Reader) ([]TestResult, error) {
	var ts []testing.Test
	if err := json.NewDecoder(r).Decode(&ts); err != nil {
		return nil, err
	}
	results := make([]TestResult, len(ts))
	for i := 0; i < len(ts); i++ {
		results[i].Test = ts[i]
	}
	return results, nil
}

// unmatchedTestPatterns returns any glob test patterns in the supplied slice
// that failed to match any tests in results.
func unmatchedTestPatterns(patterns []string, results []TestResult) []string {
	// TODO(derat): Consider also checking attribute expressions.
	if bundle.GetTestPatternType(patterns) != bundle.TestPatternGlobs {
		return nil
	}

	var unmatched []string
	for _, p := range patterns {
		re, err := testing.NewTestGlobRegexp(p)
		if err != nil {
			// Ignore bad globs -- these should've been caught earlier by the test bundles.
			continue
		}

		matched := false
		for _, res := range results {
			if re.MatchString(res.Test.Name) {
				matched = true
				break
			}
		}
		if !matched {
			unmatched = append(unmatched, p)
		}
	}
	return unmatched
}
