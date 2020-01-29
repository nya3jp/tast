// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
	"chromiumos/tast/testutil"
	"chromiumos/tast/timing"
)

// noOpCopyAndRemove can be passed to readTestOutput by tests.
func noOpCopyAndRemove(testName, dst string) error { return nil }

// readStreamedResults decodes newline-terminated, JSON-marshaled TestResult structs from r.
func readStreamedResults(t *gotesting.T, r io.Reader) []TestResult {
	var results []TestResult
	dec := json.NewDecoder(r)
	for dec.More() {
		res := TestResult{}
		if err := dec.Decode(&res); err != nil {
			t.Errorf("Failed to decode result: %v", err)
		}
		results = append(results, res)
	}
	return results
}

// testResultsEqual returns true if a and b are equivalent.
// Time fields in TestError structs are ignored, as time.Now is used
// to generate timestamps for certain types of errors.
func testResultsEqual(a, b []TestResult) bool {
	return cmp.Equal(a, b, cmpopts.IgnoreUnexported(TestResult{}),
		cmpopts.IgnoreFields(TestError{}, "Time"), cmp.AllowUnexported(hwdep.Deps{}))
}

func TestReadTestOutput(t *gotesting.T) {
	const (
		runLogText = "Here's a run log message"

		test1Name    = "foo.FirstTest"
		test1Desc    = "First description"
		test1LogText = "Here's a test log message"
		test1OutFile = testLogFilename // conflicts with test log
		test1OutData = "Data created by first test"

		test2Name        = "foo.SecondTest"
		test2Desc        = "Second description"
		test2ErrorReason = "Everything is broken :-("
		test2ErrorFile   = "some_test.go"
		test2ErrorLine   = 123
		test2ErrorStack  = "[stack trace]"
		test2OutFile     = "data.txt"
		test2OutData     = "Here's some data created by the test."

		test3Name = "foo.ThirdTest"
		test3Desc = "This test has missing dependencies"
	)

	runStartTime := time.Unix(1, 0)
	runLogTime := time.Unix(2, 0)
	test1StartTime := time.Unix(3, 0)
	test1LogTime := time.Unix(4, 0)
	test1EndTime := time.Unix(5, 0)
	test2StartTime := time.Unix(6, 0)
	test2ErrorTime := time.Unix(7, 0)
	test2EndTime := time.Unix(9, 0)
	test3StartTime := time.Unix(10, 0)
	test3EndTime := time.Unix(11, 0)
	runEndTime := time.Unix(12, 0)

	test3Deps := []string{"dep1", "dep2"}

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(test1Name, test1OutFile): test1OutData,
		filepath.Join(test2Name, test2OutFile): test2OutData,
	}); err != nil {
		t.Fatal(err)
	}

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: runStartTime,
		TestNames: []string{test1Name, test2Name, test3Name}, NumTests: 3})
	mw.WriteMessage(&control.RunLog{Time: runLogTime, Text: runLogText})
	mw.WriteMessage(&control.TestStart{Time: test1StartTime, Test: testing.TestInstance{Name: test1Name, Desc: test1Desc}})
	mw.WriteMessage(&control.TestLog{Time: test1LogTime, Text: test1LogText})
	mw.WriteMessage(&control.TestEnd{Time: test1EndTime, Name: test1Name})
	mw.WriteMessage(&control.TestStart{Time: test2StartTime, Test: testing.TestInstance{Name: test2Name, Desc: test2Desc}})
	mw.WriteMessage(&control.TestError{Time: test2ErrorTime, Error: testing.Error{
		Reason: test2ErrorReason, File: test2ErrorFile, Line: test2ErrorLine, Stack: test2ErrorStack}})
	mw.WriteMessage(&control.TestEnd{Time: test2EndTime, Name: test2Name})
	mw.WriteMessage(&control.TestStart{Time: test3StartTime, Test: testing.TestInstance{Name: test3Name, Desc: test3Desc}})
	mw.WriteMessage(&control.TestEnd{Time: test3EndTime, Name: test3Name, MissingSoftwareDeps: test3Deps})
	mw.WriteMessage(&control.RunEnd{Time: runEndTime, OutDir: outDir})

	var logBuf bytes.Buffer
	cfg := Config{
		Logger: logging.NewSimple(&logBuf, 0, false), // drop debug messages
		ResDir: filepath.Join(tempDir, "results"),
	}
	crf := func(testName, dst string) error {
		return os.Rename(filepath.Join(outDir, testName), dst)
	}
	results, unstartedTests, err := readTestOutput(context.Background(), &cfg, &b, crf, nil)
	if err != nil {
		t.Fatal("readTestOutput failed:", err)
	}
	if len(unstartedTests) != 0 {
		t.Errorf("readTestOutput reported unstarted tests %v", unstartedTests)
	}
	if err = WriteResults(context.Background(), &cfg, results, true); err != nil {
		t.Fatal("WriteResults failed:", err)
	}

	files, err := testutil.ReadFiles(cfg.ResDir)
	if err != nil {
		t.Fatal(err)
	}

	expRes := []TestResult{
		{
			TestInstance: testing.TestInstance{Name: test1Name, Desc: test1Desc},
			Start:        test1StartTime,
			End:          test1EndTime,
			OutDir:       filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		{
			TestInstance: testing.TestInstance{Name: test2Name, Desc: test2Desc},
			Errors: []TestError{
				{
					Time: test2ErrorTime,
					Error: testing.Error{
						Reason: test2ErrorReason,
						File:   test2ErrorFile,
						Line:   test2ErrorLine,
						Stack:  test2ErrorStack,
					},
				},
			},
			Start:  test2StartTime,
			End:    test2EndTime,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test2Name),
		},
		{
			TestInstance: testing.TestInstance{Name: test3Name, Desc: test3Desc},
			Start:        test3StartTime,
			End:          test3EndTime,
			SkipReason:   "missing SoftwareDeps: " + strings.Join(test3Deps, " "),
			OutDir:       filepath.Join(cfg.ResDir, testLogsDir, test3Name),
		},
	}
	var actRes []TestResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &actRes); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if !cmp.Equal(actRes, expRes, cmp.AllowUnexported(TestResult{}, hwdep.Deps{})) {
		t.Errorf("%v contains %+v; want %+v", resultsFilename, actRes, expRes)
	}

	// The streamed results file should contain the same set of results.
	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	if !cmp.Equal(streamRes, expRes, cmp.AllowUnexported(TestResult{}, hwdep.Deps{})) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}

	test1LogPath := filepath.Join(testLogsDir, test1Name, testLogFilename)
	if !strings.Contains(files[test1LogPath], test1LogText) {
		t.Errorf("%s contents %q don't contain log message %q", test1LogPath, files[test1LogPath], test1LogText)
	}
	// The first test's output file should be renamed since it conflicts with log.txt.
	test1OutPath := filepath.Join(testLogsDir, test1Name, test1OutFile+testOutputFileRenameExt)
	if files[test1OutPath] != test1OutData {
		t.Errorf("%s contains %q; want %q", test1OutPath, files[test1OutPath], test1OutData)
	}
	test2LogPath := filepath.Join(testLogsDir, test2Name, testLogFilename)
	if !strings.Contains(files[test2LogPath], test2ErrorReason) {
		t.Errorf("%s contents %q don't contain error message %q", test2LogPath, files[test2LogPath], test2ErrorReason)
	}
	test2OutPath := filepath.Join(testLogsDir, test2Name, test2OutFile)
	if files[test2OutPath] != test2OutData {
		t.Errorf("%s contains %q; want %q", test2OutPath, files[test2OutPath], test2OutData)
	}
	test3LogPath := filepath.Join(testLogsDir, test3Name, testLogFilename)
	if !strings.Contains(files[test3LogPath], test3Deps[0]) {
		t.Errorf("%s contents %q don't contain missing dependency %q", test3LogPath, files[test3LogPath], test3Deps[0])
	}

	// With non-verbose logging, the global log should include run and test messages and
	// failure/skip reasons but should skip stack traces.
	logData := logBuf.String()
	if !strings.Contains(logData, runLogText) {
		t.Errorf("Run log message %q not included in log %q", runLogText, logData)
	}
	if !strings.Contains(logData, test1LogText) {
		t.Errorf("Test log message %q not included in log %q", test1LogText, logData)
	}
	if !strings.Contains(logData, test2ErrorReason) {
		t.Errorf("Test error reason %q not included in log %q", test2ErrorReason, logData)
	}
	if strings.Contains(logData, test2ErrorStack) {
		t.Errorf("Test stack %q incorrectly included in log %q", test2ErrorStack, logData)
	}
	for _, dep := range test3Deps {
		if !strings.Contains(logData, dep) {
			t.Errorf("Test dependency %q not included in log %q", dep, logData)
		}
	}
}

func TestReadTestOutputTimingLog(t *gotesting.T) {
	const (
		testName1 = "pkg.Test1"
		testName2 = "pkg.Test2"
		stageName = "timing_stage"
	)

	// Attach a global timing log for readTestOutput to write to.
	globalLog := timing.NewLog()
	ctx := timing.NewContext(context.Background(), globalLog)

	// Test1 reports an empty timing.
	testLog1 := timing.NewLog()

	// Test2 reports a single stage.
	testLog2 := timing.NewLog()
	testLog2.StartTop(stageName).End()

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 2})
	mw.WriteMessage(&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: testName1}})
	mw.WriteMessage(&control.TestEnd{Time: time.Unix(3, 0), Name: testName1, TimingLog: testLog1})
	mw.WriteMessage(&control.TestStart{Time: time.Unix(4, 0), Test: testing.TestInstance{Name: testName2}})
	mw.WriteMessage(&control.TestEnd{Time: time.Unix(5, 0), Name: testName2, TimingLog: testLog2})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(6, 0)})

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir: td,
	}
	if _, _, err := readTestOutput(ctx, &cfg, &b, noOpCopyAndRemove, nil); err != nil {
		t.Fatal("readTestOutput failed: ", err)
	}

	// Check that there are stages representing the tests.
	if len(globalLog.Root.Children) != 2 {
		t.Fatalf("Got %d top-level stages; want 2", len(globalLog.Root.Children))
	}

	stage1 := globalLog.Root.Children[0]
	if stage1.Name != testName1 {
		t.Errorf("Stage 1 has name %q; want %q", stage1.Name, testName1)
	}
	if len(stage1.Children) != 0 {
		t.Errorf("Got %d stages under stage 1; want 0", len(stage1.Children))
	}
	if stage1.EndTime.IsZero() {
		t.Errorf("Stage 1 is not finished")
	}

	stage2 := globalLog.Root.Children[1]
	if stage2.Name != testName2 {
		t.Errorf("Stage 2 has name %q; want %q", stage2.Name, testName2)
	}
	if stage2.EndTime.IsZero() {
		t.Errorf("Stage 2 is not finished")
	}
	if len(stage2.Children) != 1 {
		t.Errorf("Got %d stages under stage 2; want 1", len(stage2.Children))
	} else if subStage := stage2.Children[0]; subStage.Name != stageName {
		t.Errorf("Sub-stage has name %q; want %q", subStage.Name, stageName)
	}
}

func TestPerTestLogContainsRunError(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Send a RunError control message in the middle of the test.
	const (
		testName = "pkg.Test1"
		errorMsg = "lost SSH connection to DUT"
	)
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
	mw.WriteMessage(&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: testName}})
	mw.WriteMessage(&control.RunError{Time: time.Unix(3, 0), Error: testing.Error{Reason: errorMsg}})

	cfg := Config{Logger: logging.NewSimple(&bytes.Buffer{}, 0, false), ResDir: td}
	if _, _, err := readTestOutput(context.Background(), &cfg, &b, noOpCopyAndRemove, nil); err == nil {
		t.Fatal("readTestOutput didn't report run error")
	} else if !strings.Contains(err.Error(), errorMsg) {
		t.Fatalf("readTestOutput error %q doesn't contain %q", err.Error(), errorMsg)
	}

	// The per-test log file should contain the error message: https://crbug.com/895716
	if files, err := testutil.ReadFiles(td); err != nil {
		t.Error("Failed to read result files: ", err)
	} else {
		logPath := filepath.Join(testLogsDir, testName, testLogFilename)
		if !strings.Contains(files[logPath], errorMsg) {
			t.Errorf("%s contents %q don't contain error message %q", logPath, files[logPath], errorMsg)
		}
	}
}

func TestValidateMessages(t *gotesting.T) {
	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	for _, tc := range []struct {
		desc        string
		resultNames []string
		msgs        []interface{}
	}{
		{"no RunStart", nil, []interface{}{
			&control.RunEnd{Time: time.Unix(1, 0), OutDir: ""},
		}},
		{"multiple RunStart", nil, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.RunStart{Time: time.Unix(2, 0)},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no RunEnd", nil, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0)},
		}},
		{"multiple RunEnd", nil, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"num tests mismatch", nil, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1"}},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
		}},
		{"unfinished test", []string{"test1", "test2"}, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1", "test2"}},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.TestStart{Time: time.Unix(4, 0), Test: testing.TestInstance{Name: "test2"}},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
		}},
		{"TestStart before RunStart", nil, []interface{}{
			&control.TestStart{Time: time.Unix(1, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.RunStart{Time: time.Unix(2, 0), TestNames: []string{"test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""},
		}},
		{"TestError without TestStart", nil, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.TestError{Time: time.Unix(2, 0), Error: testing.Error{}},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"wrong TestEnd", []string{"test1"}, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1"}},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test2"},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no TestEnd", []string{"test1"}, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1", "test2"}},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.TestStart{Time: time.Unix(3, 0), Test: testing.TestInstance{Name: "test2"}},
			&control.TestEnd{Time: time.Unix(4, 0), Name: "test2"},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
		}},
		{"TestStart with already-seen name", []string{"test1"}, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1", "test2"}},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.TestStart{Time: time.Unix(4, 0), Test: testing.TestInstance{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(5, 0), Name: "test1"},
			&control.RunEnd{Time: time.Unix(6, 0), OutDir: ""},
		}},
	} {
		b := bytes.Buffer{}
		mw := control.NewMessageWriter(&b)
		for _, msg := range tc.msgs {
			mw.WriteMessage(msg)
		}
		cfg := Config{
			Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
			ResDir: filepath.Join(tempDir, tc.desc),
		}
		if results, _, err := readTestOutput(context.Background(), &cfg, &b, noOpCopyAndRemove, nil); err == nil {
			t.Errorf("readTestOutput didn't fail for %s", tc.desc)
		} else {
			var resultNames []string
			for _, res := range results {
				resultNames = append(resultNames, res.Name)
			}
			if !reflect.DeepEqual(resultNames, tc.resultNames) {
				t.Errorf("readTestOutput for %v returned results %v; want %v", tc.desc, resultNames, tc.resultNames)
			}
		}
	}
}

func TestReadTestOutputTimeout(t *gotesting.T) {
	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	// Create a pipe, but don't write to it or close it during the test.
	// readTestOutput should time out and report an error.
	pr, pw := io.Pipe()
	defer pw.Close()

	// When the message timeout is hit, an error should be reported.
	cfg := Config{
		Logger:     logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir:     tempDir,
		msgTimeout: time.Millisecond,
	}
	if _, _, err := readTestOutput(context.Background(), &cfg, pr, noOpCopyAndRemove, nil); err == nil {
		t.Error("readTestOutput didn't return error for message timeout")
	}

	// An error should also be reported for a canceled context.
	cfg.msgTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, _, err := readTestOutput(ctx, &cfg, pr, noOpCopyAndRemove, nil); err == nil {
		t.Error("readTestOutput didn't return error for canceled context")
	}
	if elapsed := time.Now().Sub(start); elapsed >= cfg.msgTimeout {
		t.Error("readTestOutput used message timeout instead of noticing context was canceled")
	}
}

func TestWriteResultsCollectSysInfo(t *gotesting.T) {
	// This test uses types and functions from local_test.go.
	td := newLocalTestData(t)
	defer td.close()

	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		checkArgs(t, args, &runner.Args{
			Mode:           runner.CollectSysInfoMode,
			CollectSysInfo: &runner.CollectSysInfoArgs{},
		})

		json.NewEncoder(stdout).Encode(&runner.CollectSysInfoResult{})
		return 0
	}
	td.cfg.collectSysInfo = true
	td.cfg.initialSysInfo = &runner.SysInfoState{}
	if err := WriteResults(context.Background(), &td.cfg, []TestResult{}, true); err != nil {
		t.Fatal("WriteResults failed: ", err)
	}
}

func TestWriteResultsCollectSysInfoFailure(t *gotesting.T) {
	// This test uses types and functions from local_test.go.
	td := newLocalTestData(t)
	defer td.close()

	// Report an error when collecting system info.
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) { return 1 }
	td.cfg.collectSysInfo = true
	td.cfg.initialSysInfo = &runner.SysInfoState{}
	err := WriteResults(context.Background(), &td.cfg, []TestResult{}, true)
	if err == nil {
		t.Fatal("WriteResults didn't report expected error")
	}

	// The error should've been logged by WriteResults: https://crbug.com/937913
	if !strings.Contains(td.logbuf.String(), err.Error()) {
		t.Errorf("WriteResults didn't log error %q in %q", err.Error(), td.logbuf.String())
	}
}

func TestWritePartialResults(t *gotesting.T) {
	const (
		test1Name    = "pkg.Test1"
		test2Name    = "pkg.Test2"
		test3Name    = "pkg.Test3"
		test4Name    = "pkg.Test4"
		test2Reason  = "reason for error"
		test1OutFile = "test1.txt"
		test2OutFile = "test2.txt"
		test4OutFile = "test4.txt"
	)
	run1Start := time.Unix(1, 0)
	test1Start := time.Unix(2, 0)
	test1End := time.Unix(3, 0)
	test2Start := time.Unix(4, 0)
	test2Error := time.Unix(4, 100)
	run2Start := time.Unix(5, 0)
	test4Start := time.Unix(6, 0)
	test4End := time.Unix(7, 0)
	run2End := time.Unix(8, 0)

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(test1Name, test1OutFile): "",
		filepath.Join(test2Name, test2OutFile): "",
		filepath.Join(test4Name, test4OutFile): "",
	}); err != nil {
		t.Fatal(err)
	}

	// Make the runner output end abruptly without a TestEnd control message for the second test,
	// and without any messages for the third test.
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: run1Start, TestNames: []string{test1Name, test2Name, test3Name}})
	mw.WriteMessage(&control.TestStart{Time: test1Start, Test: testing.TestInstance{Name: test1Name}})
	mw.WriteMessage(&control.TestEnd{Time: test1End, Name: test1Name})
	mw.WriteMessage(&control.TestStart{Time: test2Start, Test: testing.TestInstance{Name: test2Name}})
	mw.WriteMessage(&control.TestError{Time: test2Error, Error: testing.Error{Reason: test2Reason}})

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	crf := func(testName, dst string) error {
		return os.Rename(filepath.Join(outDir, testName), dst)
	}
	results, unstarted, err := readTestOutput(context.Background(), &cfg, &b, crf, nil)
	if err == nil {
		t.Fatal("readTestOutput unexpectedly succeeded")
	}
	if expUnstarted := []string{test3Name}; !reflect.DeepEqual(unstarted, expUnstarted) {
		t.Errorf("readTestOutput returned unstarted tests %v; want %v", unstarted, expUnstarted)
	}
	files, err := testutil.ReadFiles(cfg.ResDir)
	if err != nil {
		t.Fatal(err)
	}
	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	expRes := []TestResult{
		{
			TestInstance: testing.TestInstance{Name: test1Name},
			Start:        test1Start,
			End:          test1End,
			OutDir:       filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		// No TestEnd message was received for the second test, so its entry in the streamed results
		// file should have an empty end time. The error should be included, though.
		{
			TestInstance: testing.TestInstance{Name: test2Name},
			Start:        test2Start,
			Errors: []TestError{
				{Error: testing.Error{Reason: test2Reason}},
				{Error: testing.Error{Reason: incompleteTestMsg}},
			},
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test2Name),
		},
	}
	if !testResultsEqual(streamRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}

	// The returned results should contain the same data.
	if !testResultsEqual(results, expRes) {
		t.Errorf("Returned results contain contain %+v; want %+v", results, expRes)
	}

	// Output files should be saved for finished tests.
	if _, ok := files[filepath.Join(testLogsDir, test1Name, test1OutFile)]; !ok {
		t.Errorf("Output file for %s was not saved", test1Name)
	}
	if _, ok := files[filepath.Join(testLogsDir, test2Name, test2OutFile)]; ok {
		t.Errorf("Output file for %s was saved unexpectedly", test2Name)
	}

	// Write control messages describing another run containing the third test.
	b.Reset()
	mw.WriteMessage(&control.RunStart{Time: run2Start, TestNames: []string{test4Name}})
	mw.WriteMessage(&control.TestStart{Time: test4Start, Test: testing.TestInstance{Name: test4Name}})
	mw.WriteMessage(&control.TestEnd{Time: test4End, Name: test4Name})
	mw.WriteMessage(&control.RunEnd{Time: run2End})

	// The results for the third test should be appended to the existing streamed results file.
	if _, _, err := readTestOutput(context.Background(), &cfg, &b, crf, nil); err != nil {
		t.Error("readTestOutput failed: ", err)
	}
	if files, err = testutil.ReadFiles(cfg.ResDir); err != nil {
		t.Fatal(err)
	}
	streamRes = readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	expRes = append(expRes, TestResult{
		TestInstance: testing.TestInstance{Name: test4Name},
		Start:        test4Start,
		End:          test4End,
		OutDir:       filepath.Join(cfg.ResDir, testLogsDir, test4Name),
	})
	if !testResultsEqual(streamRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}

	// Output files for the earlier run should not be clobbered.
	if _, ok := files[filepath.Join(testLogsDir, test1Name, test1OutFile)]; !ok {
		t.Errorf("Output file for %s was clobbered", test1Name)
	}
	// Output files should be saved for finished tests.
	if _, ok := files[filepath.Join(testLogsDir, test4Name, test4OutFile)]; !ok {
		t.Errorf("Output file for %s was not saved", test4Name)
	}
}

func TestUnfinishedTest(t *gotesting.T) {
	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	tm := time.Unix(1, 0) // arbitrary time to use for all control messages
	const (
		testName = "pkg.Test"
		testMsg  = "Test reported error"
		runMsg   = "Run reported error"
		runFile  = "foo.go"
		runLine  = 12
		diagMsg  = "SSH connection was lost"
	)
	incompleteErr := TestError{Error: testing.Error{Reason: incompleteTestMsg}}
	testErr := TestError{Error: testing.Error{Reason: testMsg}}
	runReason := fmt.Sprintf("Got global error: %s:%d: %s", runFile, runLine, runMsg)
	runErr := TestError{Error: testing.Error{Reason: runReason}}
	diagErr := TestError{Error: testing.Error{Reason: diagMsg}}

	// diagnoseRunErrorFunc implementations.
	emptyDiag := func(context.Context, string) string { return "" }
	goodDiag := func(context.Context, string) string { return diagMsg }

	for i, tc := range []struct {
		writeTestErr bool // write a TestError control message with testMsg
		writeRunErr  bool // write a RunError control message with runMsg
		diagFunc     diagnoseRunErrorFunc
		expErrs      []TestError
	}{
		{false, false, nil, []TestError{incompleteErr}},                      // no test or run error
		{true, false, nil, []TestError{testErr, incompleteErr}},              // test error reported
		{false, true, nil, []TestError{runErr, incompleteErr}},               // run error attributed to test
		{true, true, nil, []TestError{testErr, runErr, incompleteErr}},       // test error reported, then run error
		{true, true, emptyDiag, []TestError{testErr, runErr, incompleteErr}}, // failed diagnosis, so report run error
		{true, true, goodDiag, []TestError{testErr, diagErr, incompleteErr}}, // successful diagnosis replaces run error
	} {
		// Report that the test started but didn't finish.
		b := bytes.Buffer{}
		mw := control.NewMessageWriter(&b)
		mw.WriteMessage(&control.RunStart{Time: tm, NumTests: 1})
		mw.WriteMessage(&control.TestStart{Time: tm, Test: testing.TestInstance{Name: testName}})
		if tc.writeTestErr {
			mw.WriteMessage(&control.TestError{Time: tm, Error: testing.Error{Reason: testMsg}})
		}
		if tc.writeRunErr {
			mw.WriteMessage(&control.RunError{Time: tm, Error: testing.Error{Reason: runMsg, File: runFile, Line: runLine}})
		}

		cfg := Config{
			Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
			ResDir: filepath.Join(tempDir, strconv.Itoa(i)),
		}
		res, _, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, tc.diagFunc)
		if err == nil {
			t.Error("readTestOutput unexpectedly succeeded")
			continue
		}
		if len(res) != 1 {
			t.Errorf("readTestOutput returned %d results; want 1: %+v", len(res), res)
			continue
		}

		if res[0].Start != tm {
			t.Errorf("readTestOutput returned start time %v; want %v", res[0].Start, tm)
		}
		if !res[0].End.IsZero() {
			t.Errorf("readTestOutput returned non-zero end time %v", res[0].End)
		}
		// Ignore timestamps since run errors contain time.Now.
		if !cmp.Equal(res[0].Errors, tc.expErrs, cmpopts.IgnoreFields(TestError{}, "Time"), cmp.AllowUnexported(hwdep.Deps{})) {
			t.Errorf("readTestOutput returned errors %+v; want %+v", res[0].Errors, tc.expErrs)
		}
	}
}

func TestWriteResultsUnmatchedGlobs(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	baseCfg := NewConfig(RunTestsMode, td, td)
	baseCfg.ResDir = td

	// Report that two tests were executed.
	results := []TestResult{
		TestResult{TestInstance: testing.TestInstance{Name: "pkg.Test1"}},
		TestResult{TestInstance: testing.TestInstance{Name: "pkg.Test2"}},
	}

	// This matches the message logged by WriteResults followed by patterns that
	// are each indented by two spaces.
	re := regexp.MustCompile(
		`One or more test patterns did not match any tests:\n((?:  [^\n]+\n)+)`)

	for _, tc := range []struct {
		patterns  []string // requested test patterns
		complete  bool     // whether run was complete
		unmatched []string // expected unmatched patterns; nil if none expected
	}{
		{[]string{"pkg.Test1", "pkg.Test2"}, true, nil},                 // multiple exacts match
		{[]string{"pkg.*1", "pkg.*2"}, true, nil},                       // multiple globs match
		{[]string{"pkg.Test*"}, true, nil},                              // single glob matches
		{[]string{"pkg.Missing"}, true, []string{"pkg.Missing"}},        // single exact fails
		{[]string{"foo", "bar"}, true, []string{"foo", "bar"}},          // multiple exacts fail
		{[]string{"pkg.Test1", "pkg.Foo*"}, true, []string{"pkg.Foo*"}}, // exact matches, glob fails
		{[]string{"pkg.*", "foo.Bar"}, false, nil},                      // missing glob, but run incomplete
	} {
		cfg := *baseCfg
		out := &bytes.Buffer{}
		cfg.Logger = logging.NewSimple(out, 0, false)
		cfg.Patterns = tc.patterns
		if err := WriteResults(context.Background(), &cfg, results, tc.complete); err != nil {
			t.Errorf("WriteResults() failed for %v: %v", cfg.Patterns, err)
			continue
		}

		var unmatched []string
		if ms := re.FindStringSubmatch(out.String()); ms != nil {
			for _, ln := range strings.Split(strings.TrimRight(ms[1], "\n"), "\n") {
				unmatched = append(unmatched, ln[2:])
			}
		}
		if !reflect.DeepEqual(unmatched, tc.unmatched) {
			t.Errorf("WriteResults() with patterns %v and complete=%v logged unmatched patterns %v; want %v",
				tc.patterns, tc.complete, unmatched, tc.unmatched)
		}
	}
}
