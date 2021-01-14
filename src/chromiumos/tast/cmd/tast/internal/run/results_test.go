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
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/testutil"
)

// noOpCopyAndRemove can be passed to readTestOutput by tests.
func noOpCopyAndRemove(testName, dst string) error { return nil }

// readStreamedResults decodes newline-terminated, JSON-marshaled EntityResult structs from r.
func readStreamedResults(t *gotesting.T, r io.Reader) []*EntityResult {
	var results []*EntityResult
	dec := json.NewDecoder(r)
	for dec.More() {
		res := &EntityResult{}
		if err := dec.Decode(res); err != nil {
			t.Errorf("Failed to decode result: %v", err)
		}
		results = append(results, res)
	}
	return results
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

	const skipReason = "weather is not good"

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
	mw.WriteMessage(&control.RunStart{Time: runStartTime, TestNames: []string{test1Name, test2Name, test3Name}, NumTests: 3})
	mw.WriteMessage(&control.RunLog{Time: runLogTime, Text: runLogText})
	mw.WriteMessage(&control.EntityStart{Time: test1StartTime, Info: testing.EntityInfo{Name: test1Name, Desc: test1Desc}, OutDir: filepath.Join(outDir, test1Name)})
	mw.WriteMessage(&control.EntityLog{Time: test1LogTime, Name: test1Name, Text: test1LogText})
	mw.WriteMessage(&control.EntityEnd{Time: test1EndTime, Name: test1Name})
	mw.WriteMessage(&control.EntityStart{Time: test2StartTime, Info: testing.EntityInfo{Name: test2Name, Desc: test2Desc}, OutDir: filepath.Join(outDir, test2Name)})
	mw.WriteMessage(&control.EntityError{Time: test2ErrorTime, Name: test2Name, Error: testing.Error{Reason: test2ErrorReason, File: test2ErrorFile, Line: test2ErrorLine, Stack: test2ErrorStack}})
	mw.WriteMessage(&control.EntityEnd{Time: test2EndTime, Name: test2Name})
	mw.WriteMessage(&control.EntityStart{Time: test3StartTime, Info: testing.EntityInfo{Name: test3Name, Desc: test3Desc}})
	mw.WriteMessage(&control.EntityEnd{Time: test3EndTime, Name: test3Name, SkipReasons: []string{skipReason}})
	mw.WriteMessage(&control.RunEnd{Time: runEndTime, OutDir: outDir})

	var logBuf bytes.Buffer
	cfg := Config{
		Logger: logging.NewSimple(&logBuf, false, false), // drop debug messages
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, unstartedTests, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil)
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

	expRes := []*EntityResult{
		{
			EntityInfo: testing.EntityInfo{Name: test1Name, Desc: test1Desc},
			Start:      test1StartTime,
			End:        test1EndTime,
			OutDir:     filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		{
			EntityInfo: testing.EntityInfo{Name: test2Name, Desc: test2Desc},
			Errors: []EntityError{
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
			EntityInfo: testing.EntityInfo{Name: test3Name, Desc: test3Desc},
			Start:      test3StartTime,
			End:        test3EndTime,
			SkipReason: skipReason,
			OutDir:     filepath.Join(cfg.ResDir, testLogsDir, test3Name),
		},
	}
	var actRes []*EntityResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &actRes); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if diff := cmp.Diff(actRes, expRes); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", resultsFilename, diff)
	}

	// The streamed results file should contain the same set of results.
	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	if diff := cmp.Diff(streamRes, expRes); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", streamedResultsFilename, diff)
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
	if !strings.Contains(files[test3LogPath], skipReason) {
		t.Errorf("%s contents %q don't contain skip reason %q", test3LogPath, files[test3LogPath], skipReason)
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
	if !strings.Contains(logData, test2ErrorStack) {
		t.Errorf("Test stack %q not included in log %q", test2ErrorStack, logData)
	}
	if !strings.Contains(logData, skipReason) {
		t.Errorf("Skip reason %q not included in log %q", skipReason, logData)
	}
}

func TestReadTestOutputSameEntity(t *gotesting.T) {
	const (
		fixtName     = "foo.Fixture"
		fixt1OutDir  = fixtName + ".tmp1"
		fixt2OutDir  = fixtName + ".tmp2"
		fixt1OutFile = "out1.txt"
		fixt2OutFile = "out2.txt"
		fixt1OutData = "Output from 1st run"
		fixt2OutData = "Output from 2nd run"
	)

	epoch := time.Unix(0, 0)

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(fixt1OutDir, fixt1OutFile): fixt1OutData,
		filepath.Join(fixt2OutDir, fixt2OutFile): fixt2OutData,
	}); err != nil {
		t.Fatal(err)
	}

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: epoch})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixtName, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt1OutDir)})
	mw.WriteMessage(&control.EntityEnd{Time: epoch, Name: fixtName})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixtName, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt2OutDir)})
	mw.WriteMessage(&control.EntityEnd{Time: epoch, Name: fixtName})
	mw.WriteMessage(&control.RunEnd{Time: epoch})

	var logBuf bytes.Buffer
	cfg := Config{
		Logger: logging.NewSimple(&logBuf, false, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, unstartedTests, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil)
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

	var expRes, actRes []*EntityResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &actRes); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if diff := cmp.Diff(actRes, expRes); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", resultsFilename, diff)
	}

	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	if diff := cmp.Diff(streamRes, expRes); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", streamedResultsFilename, diff)
	}

	fixt1OutPath := filepath.Join(fixtureLogsDir, fixtName, fixt1OutFile)
	if files[fixt1OutPath] != fixt1OutData {
		t.Errorf("%s contains %q; want %q", fixt1OutPath, files[fixt1OutPath], fixt1OutData)
	}
	fixt2OutPath := filepath.Join(fixtureLogsDir, fixtName+".1", fixt2OutFile)
	if files[fixt2OutPath] != fixt2OutData {
		t.Errorf("%s contains %q; want %q", fixt2OutPath, files[fixt2OutPath], fixt2OutData)
	}
}

func TestReadTestOutputConcurrentEntity(t *gotesting.T) {
	const (
		fixt1Name    = "foo.Fixture1"
		fixt2Name    = "foo.Fixture2"
		fixt1LogText = "Log from fixture 1"
		fixt2ErrText = "Error from fixture 2"
		fixt1OutFile = "out1.txt"
		fixt2OutFile = "out2.txt"
		fixt1OutData = "Output from fixture 1"
		fixt2OutData = "Output from fixture 2"
	)

	epoch := time.Unix(0, 0)

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := testutil.WriteFiles(outDir, map[string]string{
		filepath.Join(fixt1Name, fixt1OutFile): fixt1OutData,
		filepath.Join(fixt2Name, fixt2OutFile): fixt2OutData,
	}); err != nil {
		t.Fatal(err)
	}

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: epoch})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixt1Name, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt1Name)})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixt2Name, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt2Name)})
	mw.WriteMessage(&control.EntityLog{Time: epoch, Name: fixt1Name, Text: fixt1LogText})
	mw.WriteMessage(&control.EntityError{Time: epoch, Name: fixt2Name, Error: testing.Error{Reason: fixt2ErrText}})
	mw.WriteMessage(&control.EntityEnd{Time: epoch, Name: fixt2Name})
	mw.WriteMessage(&control.EntityEnd{Time: epoch, Name: fixt1Name})
	mw.WriteMessage(&control.RunEnd{Time: epoch})

	var logBuf bytes.Buffer
	cfg := Config{
		Logger: logging.NewSimple(&logBuf, false, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, unstartedTests, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil)
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

	var expRes, actRes []*EntityResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &actRes); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if diff := cmp.Diff(actRes, expRes); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", resultsFilename, diff)
	}

	fixt1OutPath := filepath.Join(fixtureLogsDir, fixt1Name, fixt1OutFile)
	if files[fixt1OutPath] != fixt1OutData {
		t.Errorf("%s contains %q; want %q", fixt1OutPath, files[fixt1OutPath], fixt1OutData)
	}
	fixt2OutPath := filepath.Join(fixtureLogsDir, fixt2Name, fixt2OutFile)
	if files[fixt2OutPath] != fixt2OutData {
		t.Errorf("%s contains %q; want %q", fixt2OutPath, files[fixt2OutPath], fixt2OutData)
	}
}

func TestReadTestOutputTimingLog(t *gotesting.T) {
	const (
		fixtName  = "fixt.A"
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
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: fixtName, Type: testing.EntityFixture}})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(3, 0), Info: testing.EntityInfo{Name: testName1}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(4, 0), Name: testName1, TimingLog: testLog1})
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(5, 0), Info: testing.EntityInfo{Name: testName2}})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(6, 0), Name: testName2, TimingLog: testLog2})
	mw.WriteMessage(&control.EntityEnd{Time: time.Unix(7, 0), Name: fixtName, TimingLog: timing.NewLog()})
	mw.WriteMessage(&control.RunEnd{Time: time.Unix(8, 0)})

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, false, false),
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

func TestReadTestOutputAbortFixture(t *gotesting.T) {
	const (
		fixt1Name   = "foo.Fixture1"
		fixt1OutDir = fixt1Name + ".tmp"
		fixt2Name   = "foo.Fixture2"
		fixt2OutDir = fixt2Name + ".tmp"
	)

	epoch := time.Unix(0, 0)

	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	outDir := filepath.Join(tempDir, "out")
	if err := os.Mkdir(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: epoch})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixt1Name, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt1OutDir)})
	mw.WriteMessage(&control.EntityStart{Time: epoch, Info: testing.EntityInfo{Name: fixt2Name, Type: testing.EntityFixture}, OutDir: filepath.Join(outDir, fixt2OutDir)})

	var logBuf bytes.Buffer
	cfg := Config{
		Logger: logging.NewSimple(&logBuf, false, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, unstartedTests, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil)
	if err == nil {
		t.Error("readTestOutput succeeded; should fail for premature abort")
	}
	if len(unstartedTests) > 0 {
		t.Errorf("readTestOutput reported unstarted tests %v", unstartedTests)
	}
	if err := WriteResults(context.Background(), &cfg, results, true); err != nil {
		t.Fatal("WriteResults failed:", err)
	}

	files, err := testutil.ReadFiles(cfg.ResDir)
	if err != nil {
		t.Fatal(err)
	}

	var want []*EntityResult // want = ([]*EntityResult)(nil); it's not equal to (interface{})(nil)
	var got []*EntityResult
	if err := json.Unmarshal([]byte(files[resultsFilename]), &got); err != nil {
		t.Errorf("Failed to decode %v: %v", resultsFilename, err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", resultsFilename, diff)
	}

	streamRes := readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	if diff := cmp.Diff(streamRes, want); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", streamedResultsFilename, diff)
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
	mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}})
	mw.WriteMessage(&control.RunError{Time: time.Unix(3, 0), Error: testing.Error{Reason: errorMsg}})

	cfg := Config{Logger: logging.NewSimple(&bytes.Buffer{}, false, false), ResDir: td}
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
		msgs        []control.Msg
	}{
		{"no RunStart", nil, []control.Msg{
			&control.RunEnd{Time: time.Unix(1, 0), OutDir: ""},
		}},
		{"multiple RunStart", nil, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.RunStart{Time: time.Unix(2, 0)},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no RunEnd", nil, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0)},
		}},
		{"multiple RunEnd", nil, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"num tests mismatch", nil, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1"}},
			&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""},
		}},
		{"unfinished test", []string{"test1", "test2"}, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1", "test2"}},
			&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: "test1"}},
			&control.EntityEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.EntityStart{Time: time.Unix(4, 0), Info: testing.EntityInfo{Name: "test2"}},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
		}},
		{"EntityStart before RunStart", nil, []control.Msg{
			&control.EntityStart{Time: time.Unix(1, 0), Info: testing.EntityInfo{Name: "test1"}},
			&control.RunStart{Time: time.Unix(2, 0), TestNames: []string{"test1"}},
			&control.EntityEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""},
		}},
		{"EntityError without EntityStart", nil, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0)},
			&control.EntityError{Time: time.Unix(2, 0), Error: testing.Error{}},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"wrong EntityEnd", []string{"test1"}, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1"}},
			&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: "test1"}},
			&control.EntityEnd{Time: time.Unix(3, 0), Name: "test2"},
			&control.RunEnd{Time: time.Unix(3, 0), OutDir: ""},
		}},
		{"no EntityEnd", []string{"test1", "test2"}, []control.Msg{
			&control.RunStart{Time: time.Unix(1, 0), TestNames: []string{"test1", "test2"}},
			&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: "test1"}},
			&control.EntityEnd{Time: time.Unix(3, 0), Name: "test1"},
			&control.EntityStart{Time: time.Unix(4, 0), Info: testing.EntityInfo{Name: "test2"}},
			&control.RunEnd{Time: time.Unix(5, 0), OutDir: ""},
		}},
	} {
		b := bytes.Buffer{}
		mw := control.NewMessageWriter(&b)
		for _, msg := range tc.msgs {
			mw.WriteMessage(msg)
		}
		cfg := Config{
			Logger: logging.NewSimple(&bytes.Buffer{}, false, false),
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
		Logger:     logging.NewSimple(&bytes.Buffer{}, false, false),
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
	if err := WriteResults(context.Background(), &td.cfg, []*EntityResult{}, true); err != nil {
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
	err := WriteResults(context.Background(), &td.cfg, []*EntityResult{}, true)
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

	// Make the runner output end abruptly without a EntityEnd control message for the second test,
	// and without any messages for the third test.
	b := bytes.Buffer{}
	mw := control.NewMessageWriter(&b)
	mw.WriteMessage(&control.RunStart{Time: run1Start, TestNames: []string{test1Name, test2Name, test3Name}})
	mw.WriteMessage(&control.EntityStart{Time: test1Start, Info: testing.EntityInfo{Name: test1Name}, OutDir: filepath.Join(outDir, test1Name)})
	mw.WriteMessage(&control.EntityEnd{Time: test1End, Name: test1Name})
	mw.WriteMessage(&control.EntityStart{Time: test2Start, Info: testing.EntityInfo{Name: test2Name}, OutDir: filepath.Join(outDir, test2Name)})
	mw.WriteMessage(&control.EntityError{Time: test2Error, Name: test2Name, Error: testing.Error{Reason: test2Reason}})

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, false, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	results, unstarted, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil)
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
	expRes := []*EntityResult{
		{
			EntityInfo: testing.EntityInfo{Name: test1Name},
			Start:      test1Start,
			End:        test1End,
			OutDir:     filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		// No EntityEnd message was received for the second test, so its entry in the streamed results
		// file should have an empty end time. The error should be included, though.
		{
			EntityInfo: testing.EntityInfo{Name: test2Name},
			Start:      test2Start,
			Errors: []EntityError{
				{Error: testing.Error{Reason: test2Reason}},
				{Error: testing.Error{Reason: incompleteTestMsg}},
			},
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test2Name),
		},
	}
	if diff := cmp.Diff(streamRes, expRes, cmpopts.IgnoreFields(EntityError{}, "Time")); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", streamedResultsFilename, diff)
	}

	// The returned results should contain the same data.
	if diff := cmp.Diff(results, expRes, cmpopts.IgnoreFields(EntityError{}, "Time")); diff != "" {
		t.Errorf("Returned results mismatch (-got +want):\n%s", diff)
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
	mw.WriteMessage(&control.EntityStart{Time: test4Start, Info: testing.EntityInfo{Name: test4Name}, OutDir: filepath.Join(outDir, test4Name)})
	mw.WriteMessage(&control.EntityEnd{Time: test4End, Name: test4Name})
	mw.WriteMessage(&control.RunEnd{Time: run2End})

	// The results for the third test should be appended to the existing streamed results file.
	if _, _, err := readTestOutput(context.Background(), &cfg, &b, os.Rename, nil); err != nil {
		t.Error("readTestOutput failed: ", err)
	}
	if files, err = testutil.ReadFiles(cfg.ResDir); err != nil {
		t.Fatal(err)
	}
	streamRes = readStreamedResults(t, bytes.NewBufferString(files[streamedResultsFilename]))
	expRes = append(expRes, &EntityResult{
		EntityInfo: testing.EntityInfo{Name: test4Name},
		Start:      test4Start,
		End:        test4End,
		OutDir:     filepath.Join(cfg.ResDir, testLogsDir, test4Name),
	})
	if diff := cmp.Diff(streamRes, expRes, cmpopts.IgnoreFields(EntityError{}, "Time")); diff != "" {
		t.Errorf("%v mismatch (-got +want):\n%s", streamedResultsFilename, diff)
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
	incompleteErr := EntityError{Error: testing.Error{Reason: incompleteTestMsg}}
	testErr := EntityError{Error: testing.Error{Reason: testMsg}}
	runReason := fmt.Sprintf("Got global error: %s:%d: %s", runFile, runLine, runMsg)
	runErr := EntityError{Error: testing.Error{Reason: runReason}}
	diagErr := EntityError{Error: testing.Error{Reason: diagMsg}}

	// diagnoseRunErrorFunc implementations.
	emptyDiag := func(context.Context, string) string { return "" }
	goodDiag := func(context.Context, string) string { return diagMsg }

	for i, tc := range []struct {
		writeTestErr bool // write a EntityError control message with testMsg
		writeRunErr  bool // write a RunError control message with runMsg
		diagFunc     diagnoseRunErrorFunc
		expErrs      []EntityError
	}{
		{false, false, nil, []EntityError{incompleteErr}},                      // no test or run error
		{true, false, nil, []EntityError{testErr, incompleteErr}},              // test error reported
		{false, true, nil, []EntityError{runErr, incompleteErr}},               // run error attributed to test
		{true, true, nil, []EntityError{testErr, runErr, incompleteErr}},       // test error reported, then run error
		{true, true, emptyDiag, []EntityError{testErr, runErr, incompleteErr}}, // failed diagnosis, so report run error
		{true, true, goodDiag, []EntityError{testErr, diagErr, incompleteErr}}, // successful diagnosis replaces run error
	} {
		// Report that the test started but didn't finish.
		b := bytes.Buffer{}
		mw := control.NewMessageWriter(&b)
		mw.WriteMessage(&control.RunStart{Time: tm, NumTests: 1})
		mw.WriteMessage(&control.EntityStart{Time: tm, Info: testing.EntityInfo{Name: testName}})
		if tc.writeTestErr {
			mw.WriteMessage(&control.EntityError{Time: tm, Name: testName, Error: testing.Error{Reason: testMsg}})
		}
		if tc.writeRunErr {
			mw.WriteMessage(&control.RunError{Time: tm, Error: testing.Error{Reason: runMsg, File: runFile, Line: runLine}})
		}

		cfg := Config{
			Logger: logging.NewSimple(&bytes.Buffer{}, false, false),
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

		if !res[0].Start.Equal(tm) {
			t.Errorf("readTestOutput returned start time %v; want %v", res[0].Start, tm)
		}
		if !res[0].End.IsZero() {
			t.Errorf("readTestOutput returned non-zero end time %v", res[0].End)
		}
		// Ignore timestamps since run errors contain time.Now.
		if !cmp.Equal(res[0].Errors, tc.expErrs, cmpopts.IgnoreFields(EntityError{}, "Time")) {
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
	results := []*EntityResult{
		{EntityInfo: testing.EntityInfo{Name: "pkg.Test1"}},
		{EntityInfo: testing.EntityInfo{Name: "pkg.Test2"}},
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
		cfg.Logger = logging.NewSimple(out, false, false)
		cfg.Patterns = tc.patterns
		for _, r := range results {
			cfg.testNames = append(cfg.testNames, r.Name)
		}
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
