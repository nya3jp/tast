// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/control"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

// noOpCopyAndRemove can be passed to readTestOutput by tests.
func noOpCopyAndRemove(src, dst string) error { return nil }

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

func TestReadTestOutput(t *gotesting.T) {
	const (
		test1Name    = "foo.FirstTest"
		test1Desc    = "First description"
		test1LogText = "Here's a log message"

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
	test1StartTime := time.Unix(2, 0)
	test1LogTime := time.Unix(3, 0)
	test1EndTime := time.Unix(4, 0)
	test2StartTime := time.Unix(5, 0)
	test2ErrorTime := time.Unix(7, 0)
	test2EndTime := time.Unix(9, 0)
	test3StartTime := time.Unix(10, 0)
	test3EndTime := time.Unix(11, 0)
	runEndTime := time.Unix(12, 0)

	test3Deps := []string{"dep1", "dep2"}

	tempDir := testutil.TempDir(t, "results_test.")
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(test2Name, test2OutFile): test2OutData,
	}); err != nil {
		t.Fatal(err)
	}

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: runStartTime, NumTests: 3})
	mw.WriteMessage(&control.TestStart{Time: test1StartTime, Test: testing.Test{Name: test1Name, Desc: test1Desc}})
	mw.WriteMessage(&control.TestLog{Time: test1LogTime, Text: test1LogText})
	mw.WriteMessage(&control.TestEnd{Time: test1EndTime, Name: test1Name})
	mw.WriteMessage(&control.TestStart{Time: test2StartTime, Test: testing.Test{Name: test2Name, Desc: test2Desc}})
	mw.WriteMessage(&control.TestError{Time: test2ErrorTime, Error: testing.Error{
		Reason: test2ErrorReason, File: test2ErrorFile, Line: test2ErrorLine, Stack: test2ErrorStack}})
	mw.WriteMessage(&control.TestEnd{Time: test2EndTime, Name: test2Name})
	mw.WriteMessage(&control.TestStart{Time: test3StartTime, Test: testing.Test{Name: test3Name, Desc: test3Desc}})
	mw.WriteMessage(&control.TestEnd{Time: test3EndTime, Name: test3Name, MissingSoftwareDeps: test3Deps})
	mw.WriteMessage(&control.RunEnd{Time: runEndTime, OutDir: outDir})

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, err := readTestOutput(context.Background(), &cfg, &b, os.Rename)
	if err != nil {
		t.Fatal("readTestOutput failed:", err)
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
			Test:   testing.Test{Name: test1Name, Desc: test1Desc},
			Start:  test1StartTime,
			End:    test1EndTime,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		{
			Test: testing.Test{Name: test2Name, Desc: test2Desc},
			Errors: []TestError{
				TestError{
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
			Test:       testing.Test{Name: test3Name, Desc: test3Desc},
			Start:      test3StartTime,
			End:        test3EndTime,
			SkipReason: "missing deps: " + strings.Join(test3Deps, " "),
			OutDir:     filepath.Join(cfg.ResDir, testLogsDir, test3Name),
		},
	}
	var actRes []TestResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &actRes); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if !reflect.DeepEqual(actRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", resultsFilename, actRes, expRes)
	}

	// The streamed results file should contain the same set of results.
	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	if !reflect.DeepEqual(streamRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}

	outPath := filepath.Join(testLogsDir, test2Name, test2OutFile)
	if files[outPath] != test2OutData {
		t.Errorf("%s contains %q; want %q", outPath, files[outPath], test2OutData)
	}

	// TODO(derat): Check more output, including run errors.
}

func TestValidateMessages(t *gotesting.T) {
	tempDir := testutil.TempDir(t, "results_test.")
	defer os.RemoveAll(tempDir)

	for _, tc := range []struct {
		desc       string
		numResults int
		msgs       []interface{}
	}{
		{"no RunStart", 0, []interface{}{
			&control.RunEnd{Time: time.Unix(1, 0), OutDir: ""},
		}},
		{"multiple RunStart", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 0},
			&control.RunStart{Time: time.Unix(2, 0), NumTests: 0},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no RunEnd", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 0},
		}},
		{"multiple RunEnd", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 0},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"num tests mismatch", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 1},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
		}},
		{"unfinished test", 1, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 1},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.Test{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.TestStart{Time: time.Unix(4, 0), Test: testing.Test{Name: "test2"}},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
		}},
		{"TestStart before RunStart", 0, []interface{}{
			&control.TestStart{Time: time.Unix(1, 0), Test: testing.Test{Name: "test1"}},
			&control.RunStart{Time: time.Unix(2, 0), NumTests: 1},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""},
		}},
		{"TestError without TestStart", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 0},
			&control.TestError{Time: time.Unix(2, 0), Error: testing.Error{}},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"wrong TestEnd", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 0},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.Test{Name: "test1"}},
			&control.TestEnd{Time: time.Unix(3, 0), Name: "test2"},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no TestEnd", 0, []interface{}{
			&control.RunStart{Time: time.Unix(1, 0), NumTests: 2},
			&control.TestStart{Time: time.Unix(2, 0), Test: testing.Test{Name: "test1"}},
			&control.TestStart{Time: time.Unix(3, 0), Test: testing.Test{Name: "test2"}},
			&control.TestEnd{Time: time.Unix(4, 0), Name: "test2"},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
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
		if results, err := readTestOutput(context.Background(), &cfg, &b, noOpCopyAndRemove); err == nil {
			t.Errorf("readTestOutput didn't fail for %s", tc.desc)
		} else if len(results) != tc.numResults {
			t.Errorf("readTestOutput gave %v result(s) for %s; want %v", len(results), tc.desc, tc.numResults)
		}
	}
}

func TestReadTestOutputTimeout(t *gotesting.T) {
	tempDir := testutil.TempDir(t, "results_test.")
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
	if _, err := readTestOutput(context.Background(), &cfg, pr, noOpCopyAndRemove); err == nil {
		t.Error("readTestOutput didn't return error for message timeout")
	}

	// An error should also be reported for a canceled context.
	cfg.msgTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := readTestOutput(ctx, &cfg, pr, noOpCopyAndRemove); err == nil {
		t.Error("readTestOutput didn't return error for canceled context")
	}
	if elapsed := time.Now().Sub(start); elapsed >= cfg.msgTimeout {
		t.Error("readTestOutput used message timeout instead of noticing context was canceled")
	}
}

func TestNextMessageTimeout(t *gotesting.T) {
	now := time.Unix(60, 0)

	for _, tc := range []struct {
		now         time.Time
		msgTimeout  time.Duration
		ctxTimeout  time.Duration
		testStart   time.Time
		testTimeout time.Duration
		exp         time.Duration
	}{
		{
			// Outside a test, and without a custom or context timeout, use the default.
			exp: defaultMsgTimeout,
		},
		{
			// If a message timeout is supplied, use it instead of default.
			msgTimeout: 5 * time.Second,
			exp:        5 * time.Second,
		},
		{
			// Mid-test, use the test's remaining time plus the normal message timeout.
			msgTimeout:  10 * time.Second,
			testStart:   now.Add(-1 * time.Second),
			testTimeout: 5 * time.Second,
			exp:         14 * time.Second,
		},
		{
			// A context timeout should cap whatever timeout would be used otherwise.
			msgTimeout: 20 * time.Second,
			ctxTimeout: 11 * time.Second,
			exp:        11 * time.Second,
		},
	} {
		ctx := context.Background()
		var cancel context.CancelFunc
		if tc.ctxTimeout != 0 {
			ctx, cancel = context.WithDeadline(ctx, now.Add(tc.ctxTimeout))
		}

		h := resultsHandler{
			ctx: ctx,
			cfg: &Config{msgTimeout: tc.msgTimeout},
		}
		if !tc.testStart.IsZero() {
			h.res = &TestResult{
				Test:             testing.Test{Timeout: tc.testTimeout},
				testStartMsgTime: tc.testStart,
			}
		}

		// Avoid printing ugly negative numbers for unset testStart fields.
		var testStartUnix int64
		if !tc.testStart.IsZero() {
			testStartUnix = tc.testStart.Unix()
		}
		if act := h.nextMessageTimeout(now); act != tc.exp {
			t.Errorf("nextMessageTimeout(%v) (msgTimeout=%v, ctxTimeout=%v testStart=%v, testTimeout=%v) = %v; want %v",
				now.Unix(), tc.msgTimeout, tc.ctxTimeout, testStartUnix, tc.testTimeout, act, tc.exp)
		}

		if cancel != nil {
			cancel()
		}
	}
}

func TestWriteResultsCollectSysInfo(t *gotesting.T) {
	// This test uses types and functions from local_test.go.
	td := newLocalTestData()
	defer td.close()

	ob := bytes.Buffer{}
	if err := json.NewEncoder(&ob).Encode(&runner.CollectSysInfoResult{}); err != nil {
		t.Fatal(err)
	}
	stdin := addLocalRunnerFakeCmd(td.srvData.Srv, 0, ob.Bytes(), nil)

	td.cfg.collectSysInfo = true
	td.cfg.initialSysInfo = &runner.SysInfoState{}
	if err := WriteResults(context.Background(), &td.cfg, []TestResult{}, true); err != nil {
		t.Fatal("WriteResults failed: ", err)
	}
	checkArgs(t, stdin, &runner.Args{
		Mode:               runner.CollectSysInfoMode,
		CollectSysInfoArgs: runner.CollectSysInfoArgs{},
	})
}

func TestWritePartialResults(t *gotesting.T) {
	const (
		test1Name = "pkg.Test1"
		test2Name = "pkg.Test2"
		test3Name = "pkg.Test3"
	)
	run1Start := time.Unix(1, 0)
	test1Start := time.Unix(2, 0)
	test1End := time.Unix(3, 0)
	test2Start := time.Unix(4, 0)
	run2Start := time.Unix(5, 0)
	test3Start := time.Unix(6, 0)
	test3End := time.Unix(7, 0)
	run2End := time.Unix(8, 0)

	tempDir := testutil.TempDir(t, "results_test.")
	defer os.RemoveAll(tempDir)

	// Make the runner output end abruptly without a TestEnd control message for the second test.
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: run1Start, NumTests: 2})
	mw.WriteMessage(&control.TestStart{Time: test1Start, Test: testing.Test{Name: test1Name}})
	mw.WriteMessage(&control.TestEnd{Time: test1End, Name: test1Name})
	mw.WriteMessage(&control.TestStart{Time: test2Start, Test: testing.Test{Name: test2Name}})

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir: tempDir,
	}
	if _, err := readTestOutput(context.Background(), &cfg, &b, os.Rename); err == nil {
		t.Error("readTestOutput unexpectedly succeeded")
	}
	files, err := testutil.ReadFiles(cfg.ResDir)
	if err != nil {
		t.Fatal(err)
	}
	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	expRes := []TestResult{
		TestResult{
			Test:   testing.Test{Name: test1Name},
			Start:  test1Start,
			End:    test1End,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		// No TestEnd message was received for the second test, so its entry in the streamed results
		// file should have an empty end time.
		TestResult{
			Test:   testing.Test{Name: test2Name},
			Start:  test2Start,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test2Name),
		},
	}
	if !reflect.DeepEqual(streamRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}

	// Write control messages describing another run containing the third test.
	b.Reset()
	mw.WriteMessage(&control.RunStart{Time: run2Start, NumTests: 1})
	mw.WriteMessage(&control.TestStart{Time: test3Start, Test: testing.Test{Name: test3Name}})
	mw.WriteMessage(&control.TestEnd{Time: test3End, Name: test3Name})
	mw.WriteMessage(&control.RunEnd{Time: run2End})

	// The results for the third test should be appended to the existing streamed results file.
	if _, err := readTestOutput(context.Background(), &cfg, &b, os.Rename); err != nil {
		t.Error("readTestOutput failed: ", err)
	}
	if files, err = testutil.ReadFiles(cfg.ResDir); err != nil {
		t.Fatal(err)
	}
	streamRes = readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	expRes = append(expRes, TestResult{
		Test:   testing.Test{Name: test3Name},
		Start:  test3Start,
		End:    test3End,
		OutDir: filepath.Join(cfg.ResDir, testLogsDir, test3Name),
	})
	if !reflect.DeepEqual(streamRes, expRes) {
		t.Errorf("%v contains %+v; want %+v", streamedResultsFilename, streamRes, expRes)
	}
}
