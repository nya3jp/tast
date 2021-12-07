// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/run/reporting"
	"chromiumos/tast/internal/run/resultsjson"
)

func TestWriteJUnitXMLResults(t *gotesting.T) {
	passedTest := resultsjson.Test{
		Name:    "example.Pass",
		Pkg:     "chromiumos/tast/local/bundles/cros/example",
		Desc:    "Passed test",
		Timeout: 2 * time.Minute,
		Bundle:  "cros",
	}
	skippedTest := passedTest
	skippedTest.Name = "example.Skip"
	skippedTest.Desc = "Skipped test"
	failedTest := passedTest
	failedTest.Name = "example.Fail"
	skippedTest.Desc = "Failed test"
	timeZone := time.FixedZone("Local", 9*60*60)
	results := []*resultsjson.Result{
		{
			Test:       passedTest,
			Errors:     nil,
			Start:      time.Date(2021, 2, 3, 19, 00, 02, 0, timeZone),
			End:        time.Date(2021, 2, 3, 19, 00, 03, 0, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Pass",
			SkipReason: "",
		},
		{
			Test:       skippedTest,
			Errors:     nil,
			Start:      time.Date(2021, 2, 3, 19, 00, 03, 0, timeZone),
			End:        time.Date(2021, 2, 3, 19, 00, 05, 0, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Skipped",
			SkipReason: "skipped by a certain reason",
		},
		{
			Test: failedTest,
			Errors: []resultsjson.Error{
				{
					Time:   time.Date(2021, 2, 3, 19, 0, 10, 0, timeZone),
					File:   "/home/user/trunk/src/platform/tast/src/chromiumos/tast/internal/planner/run.go",
					Line:   829,
					Reason: "unknown SoftwareDeps: android",
					Stack:  ``,
				},
			},
			Start: time.Date(2021, 2, 3, 19, 0, 7, 0, timeZone),
			// Start time.Time `json:"start"`
			End:        time.Date(2021, 2, 3, 19, 0, 10, 0.5e9, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Fail",
			SkipReason: "",
		},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "results.xml")

	if err := reporting.WriteJUnitXMLResults(path, results); err != nil {
		t.Fatalf("Failed to save to XML: %s", err)
	}

	x, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read XML: %v", err)
	}

	s := strings.Split(string(x), "\n")
	expected := strings.Split(
		`<testsuites>
  <testsuite tests="3" failures="1" skipped="1">
    <testcase name="example.Pass" status="run" result="completed" timestamp="2021-02-03T10:00:02Z" time="1.0"></testcase>
    <testcase name="example.Skip" status="notrun" result="skipped" timestamp="2021-02-03T10:00:03Z" time="2.0">
      <skipped message="skipped by a certain reason"></skipped>
    </testcase>
    <testcase name="example.Fail" status="run" result="completed" timestamp="2021-02-03T10:00:07Z" time="3.5">
      <failure message="unknown SoftwareDeps: android"><![CDATA[/home/user/trunk/src/platform/tast/src/chromiumos/tast/internal/planner/run.go:829
]]></failure>
    </testcase>
  </testsuite>
</testsuites>`, "\n")
	if diff := cmp.Diff(s, expected); diff != "" {
		t.Errorf("Unexpected XML output lines (-got +want):\n%s", diff)
	}
}
