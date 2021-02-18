// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"encoding/xml"
	"fmt"
)

// TestSuite is an XML element in JUnit result.
// Some fields used in JUnit XML are not generated.
// Errors: Tast only reports success or failure. All failures are reported as Failures.
// Disabled: Tast does not have functionality to disable tests. It is done upstream scheduler job.
type TestSuite struct {
	XMLName   xml.Name
	Timestamp string      `xml:"timestamp,attr,omitempty"`
	TestCase  []*TestCase `xml:"testcase"`

	Name string `xml:"name,attr,omitempty"`
	// Classname string `xml:"classname,attr"` // Fully qualified class name, including the package prefix.
	Tests int `xml:"tests,attr,omitempty"`
	// Errors and failures are not distinguished in the Tast result. Report both as failures.
	Failures int    `xml:"failures,attr,omitempty"`
	Skipped  int    `xml:"skipped,attr,omitempty"`
	Time     string `xml:"time,attr,omitempty"` // suite start time, in ISO8601
}

// TestCase is an element in JUnit XML test result.
type TestCase struct {
	// Name of the test case. Typically this is the name of the test method.
	Name      string `xml:"name,attr"`
	Result    string `xml:"result,attr,omitempty"`
	Status    string `xml:"status,attr,omitempty"` // run or notrun
	Timestamp string `xml:"timestamp,attr"`        // start time, in ISO8601
	Time      string `xml:"time,attr"`             // duration

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
	suite := TestSuite{
		XMLName: xml.Name{Local: "testsuites"},
	}
	var skipped int
	var failures int
	for _, r := range results {
		testCase := TestCase{
			Name:   r.EntityInfo.Name,
			Status: "run",
		}
		if r.SkipReason != "" {
			testCase.Skipped = &Skipped{
				Message: r.SkipReason,
			}
			skipped++
		} else if len(r.Errors) > 0 {
			testCase.Result = "completed"
			for _, e := range r.Errors {
				testCase.Failure = append(testCase.Failure, &Failure{
					Message: e.Reason,
					Details: fmt.Sprintf("%s:%d\n%s", e.File, e.Line, e.Stack),
				})
			}
			failures++
		} else {
			_ = r.EntityInfo
			testCase.Result = "completed"
		}
		testCase.Timestamp = r.Start.Format("2006-01-02Z15:04:05")
		// Decimal point is needed for distinguishing it from nanoseconds notation.
		// e.g. "1.0" for one second.
		testCase.Time = fmt.Sprintf("%.1f", r.End.Sub(r.Start).Seconds())

		// if r.End == "0001-01-01T00:00:00Z" {} // did not complete
		suite.TestCase = append(suite.TestCase, &testCase)
	}
	suite.Tests = len(results)
	suite.Skipped = skipped
	suite.Failures = failures
	if len(results) != 0 {
		suite.Name = results[0].Bundle
	}
	return xml.MarshalIndent(suite, "", "  ")
}
