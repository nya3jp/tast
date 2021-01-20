// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package result send test results to progress sink.
package result

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	v2 "go.chromium.org/chromiumos/config/go/api/test/results/v2"
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
	// TODO: decide what to do with terminate.
	if rspn.Terminate {
		return nil
	}
	return nil
}

// makeResultRequest processes a *protocol.ReportResultRequest and prepares test results for progress API to use.
func makeResultRequest(request string, result *protocol.ReportResultRequest) (resultRequest *rtd.ReportResultRequest) {
	if rr := skippedTestResult(request, result); rr != nil {
		return rr
	}
	if rr := failedTestResult(request, result); rr != nil {
		return rr
	}
	return &rtd.ReportResultRequest{
		Request: request,
		Result:  &v2.Result{State: v2.Result_SUCCEEDED},
	}
}

// MissingTestResult create a result request for a missing test.
func MissingTestResult(requestName string) *rtd.ReportResultRequest {
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
	return nil
}

func failedTestResult(requestName string, result *protocol.ReportResultRequest) *rtd.ReportResultRequest {
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
	return nil
}
