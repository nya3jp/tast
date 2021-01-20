// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package result send test results to progress sink.
package result

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	resultspb "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// MissingTestSkipReason message is used all tests that were not run by tast.
const MissingTestSkipReason = "Test was not run"

// SendTestResult reports result through progress API.
func SendTestResult(ctx context.Context, request string, psClient rtd.ProgressSinkClient, result *protocol.ReportResultRequest) error {
	resultRequest := makeResultRequest(request, result)
	return SendReqToProgressSink(ctx, psClient, resultRequest)
}

// SendReqToProgressSink sends a result request to progress sink.
func SendReqToProgressSink(ctx context.Context, psClient rtd.ProgressSinkClient, resultRequest *rtd.ReportResultRequest) error {
	rspn, err := psClient.ReportResult(ctx, resultRequest)
	if err != nil {
		return errors.Wrap(err, "failed to report result")
	}
	// TODO(crbug.com/1166946): decide what to do with terminate
	if rspn.Terminate {
		return nil
	}
	return nil
}

// makeResultRequest processes a *protocol.ReportResultRequest and prepares test results for progress API to use.
func makeResultRequest(request string, result *protocol.ReportResultRequest) (resultRequest *rtd.ReportResultRequest) {
	if result.SkipReason != "" {
		return skippedTestResult(request, result)
	}
	if len(result.Errors) > 0 {
		return failedTestResult(request, result)
	}
	return &rtd.ReportResultRequest{
		Request: request,
		Result:  &resultspb.Result{State: resultspb.Result_SUCCEEDED},
	}
}

// MissingTestResult create a result request for a missing test.
func MissingTestResult(requestName string) *rtd.ReportResultRequest {
	return &rtd.ReportResultRequest{
		Request: requestName,
		Result: &resultspb.Result{
			State: resultspb.Result_SKIPPED,
			Errors: []*resultspb.Result_Error{
				{
					Source:   resultspb.Result_Error_TEST,
					Severity: resultspb.Result_Error_WARNING,
					Details: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SkipReason": {
								Kind: &structpb.Value_StringValue{
									StringValue: MissingTestSkipReason,
								},
							},
						},
					},
				},
			},
		},
	}
}

func skippedTestResult(requestName string, result *protocol.ReportResultRequest) *rtd.ReportResultRequest {
	return &rtd.ReportResultRequest{
		Request: requestName,
		Result: &resultspb.Result{
			State: resultspb.Result_SKIPPED,
			Errors: []*resultspb.Result_Error{
				{
					Source:   resultspb.Result_Error_TEST,
					Severity: resultspb.Result_Error_WARNING,
					Details: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SkipReason": {
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

func failedTestResult(requestName string, result *protocol.ReportResultRequest) *rtd.ReportResultRequest {
	rr := rtd.ReportResultRequest{
		Request: requestName,
		Result: &resultspb.Result{
			State: resultspb.Result_FAILED,
		},
	}
	for _, e := range result.Errors {
		rr.Result.Errors = append(rr.Result.Errors, &resultspb.Result_Error{
			Source:   resultspb.Result_Error_TEST,
			Severity: resultspb.Result_Error_CRITICAL,
			Details: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"time": {
						Kind: &structpb.Value_StringValue{
							StringValue: ptypes.TimestampString(e.Time),
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
