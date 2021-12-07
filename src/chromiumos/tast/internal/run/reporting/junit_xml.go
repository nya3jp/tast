// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"time"

	"chromiumos/tast/internal/run/resultsjson"
)

// JUnitXMLFilename is a file name to be used with WriteJUnitXMLResults.
const JUnitXMLFilename = "results.xml"

// testSuites is the top leve XML element of JUnit result.
type testSuites struct {
	XMLName   xml.Name
	TestSuite testSuite `xml:"testsuite"`
}

// testSuite is an XML element in JUnit result.
// Some fields used in JUnit XML are not generated.
// Errors: Tast only reports success or failure. All failures are reported as Failures.
// Disabled: Tast does not have functionality to disable tests. It is done upstream scheduler job.
type testSuite struct {
	TestCase []*testCase `xml:"testcase"`

	Tests int `xml:"tests,attr"`
	// Errors and failures are not distinguished in the Tast result. Report both as failures.
	Failures int `xml:"failures,attr"`
	Skipped  int `xml:"skipped,attr"`
}

// testCase is an element in JUnit XML test result.
type testCase struct {
	// Name of the test case. Typically this is the name of the test method.
	Name      string `xml:"name,attr"`
	Status    string `xml:"status,attr"`         // run or notrun
	Result    string `xml:"result,attr"`         // more detailed result
	Timestamp string `xml:"timestamp,attr"`      // start time, in ISO8601
	Time      string `xml:"time,attr,omitempty"` // duration, in seconds (with a decimal point)

	Failure []*failure `xml:"failure,omitempty"`
	Skipped *skipped   `xml:"skipped,omitempty"`
}

// failure is an element in JUnit XML test result, representing a test case failure.
type failure struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
	Details string `xml:",cdata"`
}

// skipped is an element in JUnit XML test result, representing a skipped test case.
type skipped struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
}

// WriteJUnitXMLResults saves Tast test results to path in the JUnit XML format.
func WriteJUnitXMLResults(path string, results []*resultsjson.Result) error {
	suites := testSuites{
		XMLName: xml.Name{Local: "testsuites"},
		TestSuite: testSuite{
			Tests: len(results),
		},
	}
	suite := &suites.TestSuite
	var skips int
	var failures int
	for _, r := range results {
		testCase := testCase{
			Name:      r.Name,
			Timestamp: r.Start.UTC().Format(time.RFC3339),
			// Decimal point is needed for distinguishing it from nanoseconds notation.
			// e.g. "1.0" for one second.
			Time: fmt.Sprintf("%.1f", r.End.Sub(r.Start).Seconds()),
		}
		if r.SkipReason != "" {
			testCase.Status = "notrun"
			testCase.Result = "skipped"
			testCase.Skipped = &skipped{
				Message: r.SkipReason,
			}
			skips++
		} else if len(r.Errors) > 0 {
			testCase.Status = "run"
			testCase.Result = "completed"
			for _, e := range r.Errors {
				testCase.Failure = append(testCase.Failure, &failure{
					Message: e.Reason,
					Details: fmt.Sprintf("%s:%d\n%s", e.File, e.Line, e.Stack),
				})
			}
			failures++
		} else {
			testCase.Status = "run"
			testCase.Result = "completed"
		}
		suite.TestCase = append(suite.TestCase, &testCase)
	}
	suite.Skipped = skips
	suite.Failures = failures

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}
