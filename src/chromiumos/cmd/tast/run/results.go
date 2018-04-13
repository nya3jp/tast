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

	"chromiumos/cmd/tast/timing"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
)

const (
	resultsFilename = "results.json" // file in Config.ResDir containing test results
	systemLogsDir   = "system_logs"  // dir in Config.ResDir containing DUT's system logs
	crashesDir      = "crashes"      // dir in Config.ResDir containing DUT's crashes
	testLogsDir     = "tests"        // dir in Config.ResDir containing tests' dirs
	testLogFilename = "log.txt"      // file in test's dir containing its logs

	testOutputTimeFmt = "15:04:05.000" // format for timestamps attached to test output

	defaultMsgTimeout = time.Minute // default timeout for reading next control message
)

// TestResult contains the results from a single test.
// Fields are exported so they can be marshaled by the json package.
type TestResult struct {
	// Test contains basic information about the test. This is not a runnable
	// testing.Test struct; only fields that can be marshaled to JSON are set.
	testing.Test
	// Errors contains errors encountered while running the test.
	// If it is empty, the test passed.
	Errors []testing.Error `json:"errors"`
	// Start is the time at which the test started (as reported by the test binary).
	Start time.Time `json:"start"`
	// End is the time at which the test completed (as reported by the test binary).
	End time.Time `json:"end"`
	// OutDir is the directory into which test output is stored.
	OutDir string `json:"outDir"`

	testStartMsgTime time.Time // time at which TestStart control message was received
	logFile          *os.File  // test's log file
}

// WriteResults writes results (including errors) to a JSON file in the results directory.
// It additionally logs each test's status to cfg.Logger.
// If cfg.CollectSysInfo is true, system information generated on the DUT during testing
// (e.g. logs and crashes) will also be written to the results dir.
func WriteResults(ctx context.Context, cfg *Config, results []TestResult) error {
	f, err := os.Create(filepath.Join(cfg.ResDir, resultsFilename))
	if err != nil {
		return err
	}
	defer f.Close()

	// We don't want to bail out before writing test results if sys info collection fails,
	// but we'll still return the error later.
	sysInfoErr := collectSysInfo(ctx, cfg)

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
			cfg.Logger.Logf("%s  [ PASS ]", pn)
		} else {
			for i, te := range res.Errors {
				if i == 0 {
					cfg.Logger.Logf("%s  [ FAIL ] %s", pn, te.Reason)
				} else {
					cfg.Logger.Log(strings.Repeat(" ", ml+11) + te.Reason)
				}
			}
		}
	}

	cfg.Logger.Log(sep)
	cfg.Logger.Log("Results saved to ", cfg.ResDir)
	return sysInfoErr
}

// copyAndRemoveFunc copies src on a DUT to dst on the local machine and then
// removes src from the DUT.
type copyAndRemoveFunc func(src, dst string) error

// resultsHandler processes the output from a test binary.
type resultsHandler struct {
	ctx context.Context
	cfg *Config

	runStart, runEnd time.Time         // test-binary-reported times at which run started and ended
	numTests         int               // total number of tests that are expected to run
	results          []TestResult      // information about completed tests
	res              *TestResult       // information about the currently-running test
	stage            *timing.Stage     // current test's timing stage
	crf              copyAndRemoveFunc // function used to copy and remove files from DUT
}

func (r *resultsHandler) close() {
	if r.res != nil {
		r.cfg.Logger.RemoveWriter(r.res.logFile)
		r.res.logFile.Close()
	}
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
func (r *resultsHandler) handleRunStart(msg *control.RunStart) error {
	if !r.runStart.IsZero() {
		return errors.New("multiple RunStart messages")
	}
	r.runStart = msg.Time
	r.numTests = msg.NumTests
	r.setProgress("Starting testing")
	return nil
}

// handleRunLog handles RunLog control messages from test executables.
func (r *resultsHandler) handleRunLog(msg *control.RunLog) error {
	r.cfg.Logger.Debugf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleRunError handles RunError control messages from test executables.
func (r *resultsHandler) handleRunError(msg *control.RunError) error {
	// Just return an error to abort the run.
	return fmt.Errorf("%s:%d: %s",
		filepath.Base(msg.Error.File), msg.Error.Line, msg.Error.Reason)
}

// handleRunEnd handles RunEnd control messages from test executables.
func (r *resultsHandler) handleRunEnd(msg *control.RunEnd) error {
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
func (r *resultsHandler) handleTestStart(msg *control.TestStart) error {
	if r.runStart.IsZero() {
		return errors.New("no RunStart message before TestStart")
	}
	if r.res != nil {
		return fmt.Errorf("got TestStart message for %s while %s still running",
			msg.Test.Name, r.res.Name)
	}
	if tl, ok := timing.FromContext(r.ctx); ok {
		r.stage = tl.Start(msg.Test.Name)
	}

	r.res = &TestResult{
		Test:             msg.Test,
		Start:            msg.Time,
		OutDir:           r.getTestOutputDir(msg.Test.Name),
		testStartMsgTime: time.Now(),
	}

	var err error
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
func (r *resultsHandler) handleTestLog(msg *control.TestLog) error {
	r.cfg.Logger.Debugf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleTestError handles TestError control messages from test executables.
func (r *resultsHandler) handleTestError(msg *control.TestError) error {
	if r.res == nil {
		return errors.New("got TestError message while no test was running")
	}

	te := msg.Error
	if r.res.Errors == nil {
		r.res.Errors = []testing.Error{te}
	} else {
		r.res.Errors = append(r.res.Errors, te)
	}

	ts := msg.Time.Format(testOutputTimeFmt)
	r.cfg.Logger.Logf("[%s] Error at %s:%d: %s", ts, filepath.Base(te.File), te.Line, te.Reason)
	r.cfg.Logger.Debugf("[%s] Stack trace:\n%s", ts, te.Stack)
	return nil
}

// handleTestEnd handles TestEnd control messages from test executables.
func (r *resultsHandler) handleTestEnd(msg *control.TestEnd) error {
	if r.res == nil || msg.Name != r.res.Name {
		return fmt.Errorf("got TestEnd message for not-started test %s", msg.Name)
	}
	if r.stage != nil {
		r.stage.End()
	}

	r.cfg.Logger.Logf("Completed test %s in %v with %d error(s)",
		msg.Name, msg.Time.Sub(r.res.Start).Round(time.Millisecond), len(r.res.Errors))
	r.res.End = msg.Time
	r.results = append(r.results, *r.res)

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

	dirs, err := ioutil.ReadDir(srcBase)
	if err != nil {
		return err
	}
	for _, fi := range dirs {
		dst := r.getTestOutputDir(fi.Name())
		if _, err = os.Stat(dst); os.IsNotExist(err) {
			r.cfg.Logger.Log("Skipping unexpected output dir ", fi.Name())
			continue
		}

		src := filepath.Join(srcBase, fi.Name())
		files, err := ioutil.ReadDir(src)
		if err != nil {
			return err
		}
		for _, fi2 := range files {
			if err = os.Rename(filepath.Join(src, fi2.Name()), filepath.Join(dst, fi2.Name())); err != nil {
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

	// If we're in the middle of a test, add its timeout.
	if r.res != nil {
		elapsed := now.Sub(r.res.testStartMsgTime)
		if elapsed < r.res.Timeout {
			timeout += r.res.Timeout - elapsed
		}
	}

	// Now cap the timeout to the context's deadline, if any.
	ctxDeadline, ok := r.ctx.Deadline()
	if !ok {
		return timeout
	}
	if now.After(ctxDeadline) {
		return time.Duration(0)
	}
	if ctxTimeout := ctxDeadline.Sub(now); ctxTimeout < timeout {
		return ctxTimeout
	}
	return timeout
}

// handleMessage handles generic control messages from test executables.
func (r *resultsHandler) handleMessage(msg interface{}) error {
	switch v := msg.(type) {
	case *control.RunStart:
		return r.handleRunStart(v)
	case *control.RunLog:
		return r.handleRunLog(v)
	case *control.RunError:
		return r.handleRunError(v)
	case *control.RunEnd:
		return r.handleRunEnd(v)
	case *control.TestStart:
		return r.handleTestStart(v)
	case *control.TestLog:
		return r.handleTestLog(v)
	case *control.TestError:
		return r.handleTestError(v)
	case *control.TestEnd:
		return r.handleTestEnd(v)
	default:
		return errors.New("unknown message type")
	}
}

// processMessages processes control messages and errors supplied by mch and ech.
func (r *resultsHandler) processMessages(mch chan interface{}, ech chan error) error {
	for {
		timeout := r.nextMessageTimeout(time.Now())
		select {
		case msg := <-mch:
			if msg == nil {
				// If the channel is closed, we'll read the zero value.
				return nil
			}
			if err := r.handleMessage(msg); err != nil {
				return err
			}
		case err := <-ech:
			return err
		case <-time.After(timeout):
			return fmt.Errorf("timed out after waiting %v for next message", timeout)
		}
	}
}

// readMessages reads serialized control messages from r and passes them
// via mch. If an error is encountered, it is passed via ech and no more
// reads are performed. Channels are closed before returning.
func readMessages(r io.Reader, mch chan interface{}, ech chan error) {
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

// readTestOutput reads test output from r and returns the results, which should
// be passed to WriteResults.
func readTestOutput(ctx context.Context, cfg *Config, r io.Reader, crf copyAndRemoveFunc) (
	[]TestResult, error) {
	rh := resultsHandler{
		ctx:     ctx,
		cfg:     cfg,
		results: make([]TestResult, 0),
		crf:     crf,
	}
	defer rh.close()

	mch := make(chan interface{})
	ech := make(chan error)
	go readMessages(r, mch, ech)

	if err := rh.processMessages(mch, ech); err != nil {
		return rh.results, err
	}

	if rh.runEnd.IsZero() {
		return rh.results, errors.New("no RunEnd message")
	}
	if len(rh.results) != rh.numTests {
		return rh.results, fmt.Errorf("got results for %v test(s); expected %v", len(rh.results), rh.numTests)
	}

	return rh.results, nil
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
