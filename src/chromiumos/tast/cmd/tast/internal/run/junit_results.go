// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"encoding/xml"
	"fmt"
)

// TestSuites is the top leve XML element of JUnit result.
type TestSuites struct {
	XMLName   xml.Name
	TestSuite TestSuite `xml:"testsuite"`
}

// TestSuite is an XML element in JUnit result.
// Some fields used in JUnit XML are not generated.
// Errors: Tast only reports success or failure. All failures are reported as Failures.
// Disabled: Tast does not have functionality to disable tests. It is done upstream scheduler job.
type TestSuite struct {
	TestCase []*TestCase `xml:"testcase"`

	Tests int `xml:"tests,attr"`
	// Errors and failures are not distinguished in the Tast result. Report both as failures.
	Failures int `xml:"failures,attr"`
	Skipped  int `xml:"skipped,attr"`
}

// TestCase is an element in JUnit XML test result.
type TestCase struct {
	// Name of the test case. Typically this is the name of the test method.
	Name      string `xml:"name,attr"`
	Status    string `xml:"status,attr"`         // run or notrun
	Result    string `xml:"result,attr"`         // more detailed result
	Timestamp string `xml:"timestamp,attr"`      // start time, in ISO8601
	Time      string `xml:"time,attr,omitempty"` // duration, in seconds (with a decimal point)

	Failure []*Failure `xml:"failure,omitempty"`
	Skipped *Skipped   `xml:"skipped,omitempty"`
}

// Failure is an element in JUnit XML test result, representing a test case failure.
type Failure struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
	Details string `xml:",cdata"`
}

// Skipped is an element in JUnit XML test result, representing a skipped test case.
type Skipped struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
}

// ToJUnitResults marshalizes the Tast test results into JUnit XML format.
func ToJUnitResults(results []*EntityResult) ([]byte, error) {
	suites := TestSuites{
		XMLName: xml.Name{Local: "testsuites"},
		TestSuite: TestSuite{
			Tests: len(results),
		},
	}
	suite := &suites.TestSuite
	var skipped int
	var failures int
	for _, r := range results {
		testCase := TestCase{
			Name:      r.EntityInfo.Name,
			Timestamp: r.Start.UTC().Format("2006-01-02Z15:04:05"),
			// Decimal point is needed for distinguishing it from nanoseconds notation.
			// e.g. "1.0" for one second.
			Time: fmt.Sprintf("%.1f", r.End.Sub(r.Start).Seconds()),
		}
		if r.SkipReason != "" {
			testCase.Status = "notrun"
			testCase.Result = "skipped"
			testCase.Skipped = &Skipped{
				Message: r.SkipReason,
			}
			skipped++
		} else if len(r.Errors) > 0 {
			testCase.Status = "run"
			testCase.Result = "completed"
			for _, e := range r.Errors {
				testCase.Failure = append(testCase.Failure, &Failure{
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
	suite.Skipped = skipped
	suite.Failures = failures
	return xml.MarshalIndent(suites, "", "  ")
}
