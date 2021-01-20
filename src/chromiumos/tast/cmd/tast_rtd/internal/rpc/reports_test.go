// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/google/go-cmp/cmp"
	resultspb "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast_rtd/internal/fakerts"
	"chromiumos/tast/cmd/tast_rtd/internal/result"
	"chromiumos/tast/internal/protocol"
)

func TestReportsServer_LogStream(t *testing.T) {
	srv, err := NewReportsServer(0, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start Reports server: %v", err)
	}
	addr := srv.Address()
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Test that the server is started and reachable by calling a method.
	// TODO(crbug.com/1166942): Test with actual usage of LogStream.
	cl := protocol.NewReportsClient(conn)
	if _, err := cl.LogStream(context.Background()); err != nil {
		t.Fatalf("Failed at LogStream: %v", err)
	}
}

// TestReportsServer_ReportResult makes sure reports server will pass on result to progress sink.
func TestReportsServer_ReportResult(t *testing.T) {
	ctx := context.Background()
	psServer, addr, err := fakerts.StartProgressSink(ctx)
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer psServer.Stop()

	psConn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to establish connection to fake server: ", psServer)
	}
	psClient := rtd.NewProgressSinkClient(psConn)

	testsToRequests := map[string]string{
		"PassedTest":  "PassedReq",
		"FailedTest":  "FailedReq",
		"SkippedTest": "SkippedReq",
		"MissingTest": "MissingReq", // Used for testing missing test report.
	}
	testTime := ptypes.TimestampNow()

	requests := []*protocol.ReportResultRequest{
		{
			Test: "PassedTest",
		},
		{
			Test: "FailedTest",
			Errors: []*protocol.Error{
				{
					Time:   testTime,
					Reason: "intentionally failed",
					File:   "/tmp/file.go",
					Line:   21,
					Stack:  "None",
				},
			},
		},
		{
			Test:       "SkippedTest",
			SkipReason: "intentally skipped",
		},
	}

	expectedResults := []*rtd.ReportResultRequest{
		{
			Request: "PassedReq",
			Result: &resultspb.Result{
				State: resultspb.Result_SUCCEEDED,
			},
		},
		{
			Request: "FailedReq",
			Result: &resultspb.Result{
				State: resultspb.Result_FAILED,
				Errors: []*resultspb.Result_Error{
					{
						Source:   resultspb.Result_Error_TEST,
						Severity: resultspb.Result_Error_CRITICAL,
						Details: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"time": {
									Kind: &structpb.Value_StringValue{
										StringValue: ptypes.TimestampString(requests[1].Errors[0].Time),
									},
								},
								"reason": {
									Kind: &structpb.Value_StringValue{
										StringValue: requests[1].Errors[0].Reason,
									},
								},
								"file": {
									Kind: &structpb.Value_StringValue{
										StringValue: requests[1].Errors[0].File,
									},
								},
								"line": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: float64(requests[1].Errors[0].Line),
									},
								},
								"stack": {
									Kind: &structpb.Value_StringValue{
										StringValue: requests[1].Errors[0].Stack,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Request: "SkippedReq",
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
										StringValue: requests[2].SkipReason,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Request: "MissingReq",
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
										StringValue: result.MissingTestSkipReason,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Setting up reports server and client
	reportsServer, err := NewReportsServer(0, psClient, testsToRequests)
	if err != nil {
		t.Fatalf("Failed to start Reports server: %v", err)
	}
	reportsConn, err := grpc.Dial(reportsServer.Address(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer reportsConn.Close()
	reportsClient := protocol.NewReportsClient(reportsConn)

	// Testing for normal reports.
	for i, r := range requests {
		if _, err := reportsClient.ReportResult(ctx, r); err != nil {
			t.Fatalf("Failed at ReportResult: %v", err)
		}
		resultsAtSink := psServer.Results()
		if diff := cmp.Diff(resultsAtSink[i], expectedResults[i]); diff != "" {
			t.Errorf("Got unexpected argument from request %q (-got +want):\n%s", expectedResults[i].Request, diff)
		}
	}

	// Testing for missing reports.
	if err := reportsServer.SendMissingTestsReports(ctx); err != nil {
		t.Fatalf("Failed to send reports for missing tests: %v", err)
	}
	resultsAtSink := psServer.Results()
	index := len(expectedResults) - 1
	if diff := cmp.Diff(resultsAtSink[index], expectedResults[index]); diff != "" {
		t.Errorf("Got unexpected argument from request %q (-got +want):\n%s", expectedResults[index].Request, diff)
	}

}
