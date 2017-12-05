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
	gotesting "testing"
	"time"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/control"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

// noOpCopyAndRemove can be passed to readTestOutput by tests.
func noOpCopyAndRemove(src, dst string) error { return nil }

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
	)

	runStartTime := time.Unix(1, 0)
	test1StartTime := time.Unix(2, 0)
	test1LogTime := time.Unix(3, 0)
	test1EndTime := time.Unix(4, 0)
	test2StartTime := time.Unix(5, 0)
	test2ErrorTime := time.Unix(7, 0)
	test2EndTime := time.Unix(9, 0)
	runEndTime := time.Unix(10, 0)

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
	mw.WriteMessage(&control.RunStart{runStartTime, 2})
	mw.WriteMessage(&control.TestStart{test1StartTime, test1Name,
		testing.Test{Name: test1Name, Desc: test1Desc}})
	mw.WriteMessage(&control.TestLog{test1LogTime, test1LogText})
	mw.WriteMessage(&control.TestEnd{test1EndTime, test1Name})
	mw.WriteMessage(&control.TestStart{test2StartTime, test2Name,
		testing.Test{Name: test2Name, Desc: test2Desc}})
	mw.WriteMessage(&control.TestError{test2ErrorTime,
		testing.Error{test2ErrorReason, test2ErrorFile, test2ErrorLine, test2ErrorStack}})
	mw.WriteMessage(&control.TestEnd{test2EndTime, test2Name})
	mw.WriteMessage(&control.RunEnd{runEndTime, "", outDir})

	cfg := Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir: filepath.Join(tempDir, "results"),
	}
	crf := func(src, dst string) error { return os.Rename(src, dst) }
	if err := readTestOutput(context.Background(), &cfg, &b, crf); err != nil {
		t.Fatal(err)
	}
	files, err := testutil.ReadFiles(cfg.ResDir)
	if err != nil {
		t.Fatal(err)
	}

	expRes, err := json.MarshalIndent([]testResult{
		{
			Test:   testing.Test{Name: test1Name, Desc: test1Desc},
			Start:  test1StartTime,
			End:    test1EndTime,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test1Name),
		},
		{
			Test: testing.Test{Name: test2Name, Desc: test2Desc},
			Errors: []testing.Error{
				testing.Error{
					Reason: test2ErrorReason,
					File:   test2ErrorFile,
					Line:   test2ErrorLine,
					Stack:  test2ErrorStack,
				},
			},
			Start:  test2StartTime,
			End:    test2EndTime,
			OutDir: filepath.Join(cfg.ResDir, testLogsDir, test2Name),
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	expRes = append(expRes, '\n')
	if files[resultsFilename] != string(expRes) {
		t.Errorf("%s contains %q; want %q", resultsFilename, files[resultsFilename], string(expRes))
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
		desc string
		msgs []interface{}
	}{
		{"no RunStart", []interface{}{
			&control.RunEnd{time.Unix(1, 0), "", ""},
		}},
		{"multiple RunStart", []interface{}{
			&control.RunStart{time.Unix(1, 0), 0},
			&control.RunStart{time.Unix(2, 0), 0},
			&control.RunEnd{time.Unix(3, 0), "", ""},
		}},
		{"no RunEnd", []interface{}{
			&control.RunStart{time.Unix(1, 0), 0},
		}},
		{"multiple RunEnd", []interface{}{
			&control.RunStart{time.Unix(1, 0), 0},
			&control.RunEnd{time.Unix(2, 0), "", ""},
			&control.RunEnd{time.Unix(3, 0), "", ""},
		}},
		{"num tests mismatch", []interface{}{
			&control.RunStart{time.Unix(1, 0), 1},
			&control.RunEnd{time.Unix(2, 0), "", ""},
		}},
		{"unfinished test", []interface{}{
			&control.RunStart{time.Unix(1, 0), 1},
			&control.TestStart{time.Unix(2, 0), "test1", testing.Test{Name: "test1"}},
			&control.TestEnd{time.Unix(3, 0), "test1"},
			&control.TestStart{time.Unix(4, 0), "test2", testing.Test{Name: "test2"}},
			&control.RunEnd{time.Unix(5, 0), "", ""},
		}},
		{"TestStart before RunStart", []interface{}{
			&control.TestStart{time.Unix(1, 0), "test1", testing.Test{Name: "test1"}},
			&control.RunStart{time.Unix(2, 0), 1},
			&control.TestEnd{time.Unix(3, 0), "test1"},
			&control.RunEnd{time.Unix(4, 0), "", ""},
		}},
		{"TestError without TestStart", []interface{}{
			&control.RunStart{time.Unix(1, 0), 0},
			&control.TestError{time.Unix(2, 0), testing.Error{}},
			&control.RunEnd{time.Unix(3, 0), "", ""},
		}},
		{"wrong TestEnd", []interface{}{
			&control.RunStart{time.Unix(1, 0), 0},
			&control.TestStart{time.Unix(2, 0), "test1", testing.Test{Name: "test1"}},
			&control.TestEnd{time.Unix(3, 0), "test2"},
			&control.RunEnd{time.Unix(3, 0), "", ""},
		}},
		{"no TestEnd", []interface{}{
			&control.RunStart{time.Unix(1, 0), 2},
			&control.TestStart{time.Unix(2, 0), "test1", testing.Test{Name: "test1"}},
			&control.TestStart{time.Unix(3, 0), "test2", testing.Test{Name: "test2"}},
			&control.TestEnd{time.Unix(4, 0), "test2"},
			&control.RunEnd{time.Unix(5, 0), "", ""},
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
		if err := readTestOutput(context.Background(), &cfg, &b, noOpCopyAndRemove); err == nil {
			t.Errorf("readTestOutput() didn't fail for %s", tc.desc)
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

	cfg := Config{
		Logger:     logging.NewSimple(&bytes.Buffer{}, 0, false),
		ResDir:     tempDir,
		msgTimeout: time.Millisecond,
	}
	if err := readTestOutput(context.Background(), &cfg, pr, noOpCopyAndRemove); err == nil {
		t.Error("readTestOutput didn't return error for timeout")
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
			h.res = &testResult{
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
