// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	gotesting "testing"
	"time"

	"chromiumos/tast/cmd/logging"
	"chromiumos/tast/common/control"
	"chromiumos/tast/common/testing"
	"chromiumos/tast/common/testutil"
)

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

	lb := bytes.Buffer{}
	resDir := filepath.Join(tempDir, "results")
	crf := func(src, dst string) error { return os.Rename(src, dst) }
	if err := readTestOutput(context.Background(), logging.NewSimple(&lb, 0, false),
		&b, resDir, crf); err != nil {
		t.Fatal(err)
	}
	files, err := testutil.ReadFiles(resDir)
	if err != nil {
		t.Fatal(err)
	}

	expRes, err := json.MarshalIndent([]testResult{
		{
			Test:   testing.Test{Name: test1Name, Desc: test1Desc},
			Start:  test1StartTime,
			End:    test1EndTime,
			OutDir: filepath.Join(resDir, testLogsDir, test1Name),
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
			OutDir: filepath.Join(resDir, testLogsDir, test2Name),
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
