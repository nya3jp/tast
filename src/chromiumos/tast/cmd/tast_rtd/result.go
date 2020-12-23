// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	structpb "github.com/golang/protobuf/ptypes/struct"
	v2 "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"

	"chromiumos/tast/errors"
)

const (
	resultsFilename = "results.json" // File containing stream of newline-separated JSON EntityResult objects

	skipReasonKey = "skipReason" // Keyword for skip reason in results.json.
	errorsKey     = "errors"     // Keyword for errors in results.json.
)

// testErrors stores information for each error.
type testError struct {
	Time   string `json:"time"`
	Reason string `json:"reason"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Stack  string `json:"stack"`
}

// testResult stores result for one test.
type testResult struct {
	// name contains the test name.
	Name string `json:"name"`
	// Errors contains errors encountered while running the entity.
	// If it is empty, the entity passed.
	Errors []testError `json:"errors"`
	// It is empty if the test actually ran.
	SkipReason string `json:"skipReason"`
}

// sendTestResults reads resultDir/results.json and reports result through progress API.
func sendTestResults(ctx context.Context, logger *log.Logger, psClient rtd.ProgressSinkClient, inv *rtd.Invocation, resultDir string) error {
	fullPathName := filepath.Join(resultDir, resultsFilename)
	resultFile, err := os.Open(fullPathName)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %v", fullPathName)
	}
	defer resultFile.Close()
	data, err := ioutil.ReadFile(fullPathName)
	if err != nil {
		return errors.Wrapf(err, "fail to read file %v", fullPathName)
	}
	resultRequests, err := makeResultRequests(logger, inv, data)
	if err != nil {
		return errors.Wrapf(err, "fail to report test result from file %v", fullPathName)
	}
	for _, rr := range resultRequests {
		rspn, err := psClient.ReportResult(ctx, rr)
		if err != nil {
			return errors.Wrap(err, "failed to report result")
		}
		if rspn.Terminate {
			return nil
		}
	}
	return nil
}

// makeResultRequests parses the json text from testData and prepares test results for progress API to use.
func makeResultRequests(logger *log.Logger, inv *rtd.Invocation, testData []byte) (resultRequests []*rtd.ReportResultRequest, err error) {
	var results []*testResult
	if err := json.Unmarshal(testData, &results); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal test results")
	}
	// A map to maintain test names to result structures.
	testResults := make(map[string]*testResult)
	for i, r := range results {
		if r.Name == "" {
			return nil, errors.Errorf("Entry %v has empty test name: %+v", i, r)
		}
		testResults[r.Name] = results[i]
	}
	for _, request := range inv.Requests {
		result, ok := testResults[request.Test]
		if !ok {
			// TODO: Discuss what is the best way to handle this case.
			resultRequests = append(resultRequests, missingTestResult(request.Name))
			logger.Println("Did not receive test result for request ", request.Name)
			continue
		}
		if rr := skippedTestResult(request.Name, result); rr != nil {
			resultRequests = append(resultRequests, rr)
			continue
		}
		if rr := failedTestResult(request.Name, result); rr != nil {
			resultRequests = append(resultRequests, rr)
			continue
		}
		resultRequests = append(resultRequests, &rtd.ReportResultRequest{
			Request: request.Name,
			Result:  &v2.Result{State: v2.Result_SUCCEEDED},
		})
	}
	return resultRequests, nil
}

func missingTestResult(requestName string) *rtd.ReportResultRequest {
	// TODO: Discuss what is the best way to handle this case.
	return &rtd.ReportResultRequest{
		Request: requestName,
		Result: &v2.Result{
			State: v2.Result_SKIPPED,
			Errors: []*v2.Result_Error{
				{
					Source:   v2.Result_Error_TEST,
					Severity: v2.Result_Error_WARNING,
					Details: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							skipReasonKey: {
								Kind: &structpb.Value_StringValue{
									StringValue: "Test was not found",
								},
							},
						},
					},
				},
			},
		},
	}
}

func skippedTestResult(requestName string, result *testResult) *rtd.ReportResultRequest {
	if result.SkipReason != "" {
		return &rtd.ReportResultRequest{
			Request: requestName,
			Result: &v2.Result{
				State: v2.Result_SKIPPED,
				Errors: []*v2.Result_Error{
					{
						Source:   v2.Result_Error_TEST,
						Severity: v2.Result_Error_WARNING,
						Details: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								skipReasonKey: {
									Kind: &structpb.Value_StringValue{
										StringValue: result.SkipReason,
									},
								},
							},
						},
					},
				},
			},
		}
	}
	return nil
}

func failedTestResult(requestName string, result *testResult) *rtd.ReportResultRequest {
	if len(result.Errors) > 0 {
		rr := rtd.ReportResultRequest{
			Request: requestName,
			Result: &v2.Result{
				State: v2.Result_FAILED,
			},
		}
		for _, e := range result.Errors {
			rr.Result.Errors = append(rr.Result.Errors, &v2.Result_Error{
				Source:   v2.Result_Error_TEST,
				Severity: v2.Result_Error_CRITICAL,
				Details: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"time": {
							Kind: &structpb.Value_StringValue{
								StringValue: e.Time,
							},
						},
						"reason": {
							Kind: &structpb.Value_StringValue{
								StringValue: e.Reason,
							},
						},
						"file": {
							Kind: &structpb.Value_StringValue{
								StringValue: e.File,
							},
						},
						"line": {
							Kind: &structpb.Value_NumberValue{
								NumberValue: float64(e.Line),
							},
						},
						"stack": {
							Kind: &structpb.Value_StringValue{
								StringValue: e.Stack,
							},
						},
					},
				},
			})
		}
		return &rr
	}
	return nil
}
