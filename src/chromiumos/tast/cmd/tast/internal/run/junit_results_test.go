// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"strings"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/testing"
)

func TestWriteJUnitResults(t *gotesting.T) {
	passedTest := testing.EntityInfo{
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
	results := []*EntityResult{
		&EntityResult{
			EntityInfo: passedTest,
			Errors:     []EntityError{},
			Start:      time.Date(2021, 2, 3, 19, 00, 02, 0, timeZone),
			End:        time.Date(2021, 2, 3, 19, 00, 03, 0, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Pass",
			SkipReason: "",
		},
		&EntityResult{
			EntityInfo: skippedTest,
			Errors:     []EntityError{},
			Start:      time.Date(2021, 2, 3, 19, 00, 03, 0, timeZone),
			End:        time.Date(2021, 2, 3, 19, 00, 05, 0, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Skipped",
			SkipReason: "skipped by a certain reason",
		},
		&EntityResult{
			EntityInfo: failedTest,
			Errors: []EntityError{
				{
					Error: testing.Error{
						File:   "/home/user/trunk/src/platform/tast/src/chromiumos/tast/internal/planner/run.go",
						Line:   829,
						Reason: "unknown SoftwareDeps: android",
						Stack:  ``,
					},
					Time: time.Date(2021, 2, 3, 19, 0, 10, 0, timeZone),
				},
			},
			Start: time.Date(2021, 2, 3, 19, 0, 7, 0, timeZone),
			// Start time.Time `json:"start"`
			End:        time.Date(2021, 2, 3, 19, 0, 10, 0.5e9, timeZone),
			OutDir:     "/tmp/tast/results/20210203-1000/tests/example.Fail",
			SkipReason: "",
		},
	}
	x, err := ToJUnitResults(results)
	if err != nil {
		t.Fatalf("Failed to marshal to XML: %s", err)
	}
	s := strings.Split(string(x), "\n")
	expected := strings.Split(
		`<testsuites>
  <testsuite tests="3" failures="1" skipped="1">
    <testcase name="example.Pass" status="run" result="completed" timestamp="2021-02-03Z10:00:02" time="1.0"></testcase>
    <testcase name="example.Skip" status="notrun" result="skipped" timestamp="2021-02-03Z10:00:03" time="2.0">
      <skipped message="skipped by a certain reason"></skipped>
    </testcase>
    <testcase name="example.Fail" status="run" result="completed" timestamp="2021-02-03Z10:00:07" time="3.5">
      <failure message="unknown SoftwareDeps: android"><![CDATA[/home/user/trunk/src/platform/tast/src/chromiumos/tast/internal/planner/run.go:829
]]></failure>
    </testcase>
  </testsuite>
</testsuites>`, "\n")
	if diff := cmp.Diff(s, expected); diff != "" {
		t.Errorf("Unexpected XML output lines (-got +want):\n%s", diff)
	}
}
