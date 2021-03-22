// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"strings"
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
	const (
		testName1    = "test.Name1"
		logSinkName1 = "tests/name01-20210123/test.Name1/log.txt"
		requestName1 = "request_for_test_name1"
		testLog1a    = "log1a"
		testLog1b    = "log1b"
		testName2    = "test.Name2"
		requestName2 = "request_for_test_name2"
		logSinkName2 = "tests/name01-20210123/test.Name2/log.txt"
		testLog2a    = "log2a"
	)

	ctx := context.Background()
	psServer, psAddr, err := fakerts.StartProgressSink(ctx, 0)
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer psServer.Stop()

	psConn, err := grpc.Dial(psAddr.String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to establish connection to fake server: ", psServer)
	}
	psClient := rtd.NewProgressSinkClient(psConn)

	testsToRequests := map[string]string{
		testName1: requestName1,
		testName2: requestName2,
	}
	srv, err := NewReportsServer(0, psClient, testsToRequests)
	if err != nil {
		t.Fatalf("Failed to start Reports server: %v", err)
	}

	conn, err := grpc.Dial(srv.Address(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	cl := protocol.NewReportsClient(conn)
	strm, err := cl.LogStream(context.Background())
	if err != nil {
		t.Fatalf("Failed at LogStream: %v", err)
	}
	if err := strm.Send(&protocol.LogStreamRequest{
		Test:    testName1,
		LogPath: logSinkName1,
		Data:    []byte(testLog1a),
	}); err != nil {
		t.Errorf("failed to send: %v", err)
	}
	if err := strm.Send(&protocol.LogStreamRequest{
		Test:    testName1,
		LogPath: logSinkName1,
		Data:    []byte(testLog1b),
	}); err != nil {
		t.Errorf("failed to send: %v", err)
	}
	if err := strm.Send(&protocol.LogStreamRequest{
		Test:    testName2,
		LogPath: logSinkName2,
		Data:    []byte(testLog2a),
	}); err != nil {
		t.Errorf("failed to send: %v", err)
	}
	if _, err := strm.CloseAndRecv(); err != nil {
		t.Errorf("failed to CloseAndRecv: %v", err)
	}
	srv.Stop()

	if s := string(psServer.ReceivedLog(logSinkName1, requestName1)); !strings.Contains(s, testLog1a+testLog1b) {
		t.Errorf("Log sent to test #1 was not forwarded. Log#1=%q", s)
	}
	if s := string(psServer.ReceivedLog(logSinkName1, requestName1)); strings.Contains(s, testLog2a) {
		t.Errorf("Log sent to #2 appeared in #1. Log#1=%q", s)
	}
	if s := string(psServer.ReceivedLog(logSinkName2, requestName2)); strings.Contains(s, testLog1a) {
		t.Errorf("Log sent to #1 appeared in #2. Log#2=%q", s)
	}
}

// TestReportsServer_ReportResult makes sure reports server will pass on result to progress sink.
func TestReportsServer_ReportResult(t *testing.T) {
	ctx := context.Background()
	psServer, addr, err := fakerts.StartProgressSink(ctx, 0)
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
			Errors: []*protocol.ErrorReport{
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
		rspn, err := reportsClient.ReportResult(ctx, r)
		if err != nil {
			t.Fatalf("Failed at ReportResult: %v", err)
		}
		if rspn.Terminate {
			t.Errorf("ReportResult(ctx, %+v) returned true; wanted false", r)
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

// TestReportsServer_ReportResultWithTerminate makes sure reports server will pass on terminate response from progress sink to reports clients.
func TestReportsServer_ReportResultWithTerminate(t *testing.T) {
	ctx := context.Background()
	psServer, addr, err := fakerts.StartProgressSink(ctx, 1)
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
		"PassedTest": "PassedReq",
		"FailedTest": "FailedReq",
	}
	testTime := ptypes.TimestampNow()

	requests := []*protocol.ReportResultRequest{
		{
			Test: "PassedTest",
		},
		{
			Test: "FailedTest",
			Errors: []*protocol.ErrorReport{
				{
					Time:   testTime,
					Reason: "intentionally failed",
					File:   "/tmp/file.go",
					Line:   21,
					Stack:  "None",
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
	for _, r := range requests {
		rspn, err := reportsClient.ReportResult(ctx, r)
		if err != nil {
			t.Fatalf("Failed at ReportResult: %v", err)
		}
		if len(r.Errors) > 0 {
			if !rspn.Terminate {
				t.Errorf("ReportResult(ctx, %+v) returned false for ; wanted true", r)
			}
			break
		}
		if rspn.Terminate {
			t.Errorf("ReportResult(ctx, %+v) returned true for ; wanted false", r)
		}
	}
}
