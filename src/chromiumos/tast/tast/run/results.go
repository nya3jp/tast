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

	"chromiumos/tast/common/control"
	"chromiumos/tast/common/testing"
	"chromiumos/tast/tast/logging"
	"chromiumos/tast/tast/timing"
)

const (
	resultsFilename = "results.json" // file in resDir containing test results
	systemLogsDir   = "system_logs"  // dir in resDir containing DUT's system logs
	testLogsDir     = "tests"        // dir in resDir containing tests' dirs
	testLogFilename = "log.txt"      // file in test's dir containing its logs

	testOutputTimeFmt = "15:04:05.000" // format for timestamps attached to test output
)

// testResult contains the results from a single test.
// Fields are exported so they can be marshaled by the json package.
type testResult struct {
	// Name is the test's name.
	Name string `json:"name"`
	// Errors contains errors encountered while running the test.
	// If it is empty, the test passed.
	Errors []testing.Error `json:"errors"`
	// Start is the time at which the test started.
	Start time.Time `json:"start"`
	// End is the time at which the test completed.
	End time.Time `json:"end"`
	// OutDir is the directory into which test output is stored.
	OutDir string `json:"outDir"`

	logFile *os.File // test's log file
}

// copyAndRemoveFunc copies src on a DUT to dst on the local machine and then
// removes src from the DUT.
type copyAndRemoveFunc func(src, dst string) error

// resultsHandler processes the output from a test binary.
type resultsHandler struct {
	ctx context.Context
	lg  logging.Logger

	numTests int               // total number of tests that are expected to run
	results  []testResult      // information about completed tests
	res      *testResult       // information about the currently-running test
	stage    *timing.Stage     // current test's timing stage
	resDir   string            // directory into which this run's results will be written
	crf      copyAndRemoveFunc // function used to copy and remove files from DUT
}

func (r *resultsHandler) close() {
	if r.res != nil {
		r.lg.RemoveWriter(r.res.logFile)
		r.res.logFile.Close()
	}
}

// setProgress updates the currently displayed progress to display the number of completed vs.
// total tests and the message s.
func (r *resultsHandler) setProgress(s string) {
	if s != "" {
		s = " " + s
	}
	r.lg.Status(fmt.Sprintf("[%d/%d]%s", len(r.results), r.numTests, s))
}

// handleRunStart handles RunStart control messages from test executables.
func (r *resultsHandler) handleRunStart(msg *control.RunStart) error {
	r.numTests = msg.NumTests
	r.setProgress("Starting testing")
	return nil
}

// handleRunLog handles RunLog control messages from test executables.
func (r *resultsHandler) handleRunLog(msg *control.RunLog) error {
	r.lg.Debugf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleRunError handles RunError control messages from test executables.
func (r *resultsHandler) handleRunError(msg *control.RunError) error {
	// Just return an error to abort the run.
	return fmt.Errorf("%s:%d: %s", msg.Error.File, msg.Error.Line, msg.Error.Reason)
}

// handleRunEnd handles RunEnd control messages from test executables.
func (r *resultsHandler) handleRunEnd(msg *control.RunEnd) error {
	if len(msg.LogDir) != 0 {
		r.setProgress("Copying system logs")
		if err := r.crf(msg.LogDir, filepath.Join(r.resDir, systemLogsDir)); err != nil {
			r.lg.Log("Failed to copy system logs: ", err)
		}
	}

	if len(msg.OutDir) != 0 {
		r.setProgress("Copying output files")
		localOutDir := filepath.Join(r.resDir, "out.tmp")
		if err := r.crf(msg.OutDir, localOutDir); err != nil {
			r.lg.Log("Failed to copy test output data: ", err)
		} else if err := r.moveTestOutputData(localOutDir); err != nil {
			r.lg.Log("Failed to move test output data: ", err)
		}
	}

	return r.writeResults()
}

// handleTestStart handles TestStart control messages from test executables.
func (r *resultsHandler) handleTestStart(msg *control.TestStart) error {
	if r.res != nil {
		return fmt.Errorf("notified about start of %s while %s still running", msg.Name, r.res.Name)
	}
	if tl, ok := timing.FromContext(r.ctx); ok {
		r.stage = tl.Start(msg.Name)
	}
	r.res = &testResult{
		Name:   msg.Name,
		Start:  msg.Time,
		OutDir: r.getTestOutputDir(msg.Name),
	}

	var err error
	if err = os.MkdirAll(r.res.OutDir, 0755); err != nil {
		return err
	}
	if r.res.logFile, err = os.Create(
		filepath.Join(r.res.OutDir, testLogFilename)); err != nil {
		return err
	}
	if err = r.lg.AddWriter(r.res.logFile, log.LstdFlags); err != nil {
		return err
	}

	r.lg.Log("Started test ", r.res.Name)
	r.setProgress("Running " + r.res.Name)
	return nil
}

// handleTestLog handles TestLog control messages from test executables.
func (r *resultsHandler) handleTestLog(msg *control.TestLog) error {
	r.lg.Debugf("[%s] %s", msg.Time.Format(testOutputTimeFmt), msg.Text)
	return nil
}

// handleTestError handles TestError control messages from test executables.
func (r *resultsHandler) handleTestError(msg *control.TestError) error {
	if r.res == nil {
		return errors.New("notified about test error while no test was running")
	}

	te := msg.Error
	if r.res.Errors == nil {
		r.res.Errors = []testing.Error{te}
	} else {
		r.res.Errors = append(r.res.Errors, te)
	}
	r.lg.Logf("[%s] %s:%d: %s\n", msg.Time.Format(testOutputTimeFmt),
		filepath.Base(te.File), te.Line, te.Reason)
	// TODO(derat): Log te.Stack to the per-test log while not spamming verbose output.
	return nil
}

// handleTestEnd handles TestEnd control messages from test executables.
func (r *resultsHandler) handleTestEnd(msg *control.TestEnd) error {
	if r.res == nil || msg.Name != r.res.Name {
		return fmt.Errorf("notified about completion of not-started test %s", msg.Name)
	}
	if r.stage != nil {
		r.stage.End()
	}

	r.lg.Logf("Completed test %s in %0.1f sec with %d error(s)",
		msg.Name, msg.Time.Sub(r.res.Start).Seconds(), len(r.res.Errors))
	r.res.End = msg.Time
	r.results = append(r.results, *r.res)

	if err := r.lg.RemoveWriter(r.res.logFile); err != nil {
		r.lg.Log(err)
	}
	if err := r.res.logFile.Close(); err != nil {
		r.lg.Log(err)
	}
	r.res = nil
	r.stage = nil
	return nil
}

// getTestOutputDir returns the directory into which data should be stored for a test named testName.
func (r *resultsHandler) getTestOutputDir(testName string) string {
	return filepath.Join(r.resDir, testLogsDir, testName)
}

// moveTestOutputData moves per-test output data from test-named directories under srcBase
// to the corresponding test directories under r.resDir.
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
			r.lg.Log("Skipping unexpected output dir ", fi.Name())
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
		r.lg.Log("Failed to remove temp dir: ", err)
	}
	return nil
}

// writeResults writes the test results (including errors) to a machine-readable text file
// in the results directory. It additionally logs the results.
func (r *resultsHandler) writeResults() error {
	f, err := os.Create(filepath.Join(r.resDir, resultsFilename))
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(r.results); err != nil {
		return err
	}

	ml := 0
	for _, res := range r.results {
		if len(res.Name) > ml {
			ml = len(res.Name)
		}
	}

	sep := strings.Repeat("-", 80)
	r.lg.Log(sep)

	for _, res := range r.results {
		pn := fmt.Sprintf("%-"+strconv.Itoa(ml)+"s", res.Name)
		if len(res.Errors) == 0 {
			r.lg.Logf("%s  [ PASS ]", pn)
		} else {
			for i, te := range res.Errors {
				if i == 0 {
					r.lg.Logf("%s  [ FAIL ] %s", pn, te.Reason)
				} else {
					r.lg.Log(strings.Repeat(" ", ml+11) + te.Reason)
				}
			}
		}
	}

	r.lg.Log(sep)
	r.lg.Logf("Results saved to %s", r.resDir)
	return nil
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

// readTestOutput reads test output from r and writes the test results to resDir.
func readTestOutput(ctx context.Context, lg logging.Logger, r io.Reader,
	resDir string, crf copyAndRemoveFunc) error {
	rh := resultsHandler{
		ctx:     ctx,
		lg:      lg,
		results: make([]testResult, 0),
		resDir:  resDir,
		crf:     crf,
	}
	defer rh.close()

	mr := control.NewMessageReader(r)
	for mr.More() {
		msg, err := mr.ReadMessage()
		if err != nil {
			return err
		}
		if err = rh.handleMessage(msg); err != nil {
			return err
		}
	}
	return nil
}
