// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakerts

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	resultspb "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"
)

func TestProgressSink_ReportLog(t *testing.T) {
	srv, addr, err := StartProgressSink(context.Background(), 0)
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer srv.Stop()

	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to establish connection to fake server: ", srv)
	}
	client := rtd.NewProgressSinkClient(conn)
	stream, err := client.ReportLog(context.Background())
	if err != nil {
		t.Fatal("Failed to call ReportLog: ", err)
	}

	const name1 = "/tmp/result/name1"
	const request1 = "req0001"
	const name2 = "/tmp/result/name2"
	const request2 = "req0002"
	req1 := rtd.ReportLogRequest{
		Name:    name1,
		Request: request1,
	}
	req1.Data = []byte("Hello, ")
	if err := stream.Send(&req1); err != nil {
		t.Fatal("Failed to send to ReportLog stream: ", err)
	}
	req1.Data = []byte("world!")
	if err := stream.Send(&req1); err != nil {
		t.Fatal("Failed to send to ReportLog stream: ", err)
	}
	// Test different name / request combinations
	req2 := rtd.ReportLogRequest{
		Name:    name1,
		Request: request2,
		Data:    []byte("#2"),
	}
	if err := stream.Send(&req2); err != nil {
		t.Fatal("Failed to send to ReportLog stream: ", err)
	}
	req3 := rtd.ReportLogRequest{
		Name:    name2,
		Request: request1,
		Data:    []byte("#3"),
	}
	if err := stream.Send(&req3); err != nil {
		t.Fatal("Failed to send to ReportLog stream: ", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatal("Failed at CloseAndRecv(): ", err)
	}

	actual := srv.ReceivedLog(name1, request1)
	expected := []byte("Hello, world!")
	if cmp.Diff(actual, expected) != "" {
		t.Errorf("got %q, want %q", actual, expected)
	}
	actual = srv.ReceivedLog(name1, request2)
	expected = []byte("#2")
	if cmp.Diff(actual, expected) != "" {
		t.Errorf("got %q, want %q", actual, expected)
	}
	actual = srv.ReceivedLog(name2, request1)
	expected = []byte("#3")
	if cmp.Diff(actual, expected) != "" {
		t.Errorf("got %q, want %q", actual, expected)
	}
}

// TestProgressSink_ReportResult makes sure the fake progress sink can handle ReportResult API.
func TestProgressSink_ReportResult(t *testing.T) {
	srv, addr, err := StartProgressSink(context.Background(), 0)
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer srv.Stop()

	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to establish connection to fake server: ", srv)
	}
	client := rtd.NewProgressSinkClient(conn)
	request := rtd.ReportResultRequest{
		Request: "PassedReq",
		Result: &resultspb.Result{
			State: resultspb.Result_SUCCEEDED,
		},
	}
	if _, err = client.ReportResult(context.Background(), &request); err != nil {
		t.Fatal("Failed to call ReportResult: ", err)
	}
	results := srv.Results()
	if results[0].Request != request.Request {
		t.Errorf("Got unexpected result request name -got %q +want %q", results[0].Request, request.Request)
	}
	if results[0].Result.State != request.Result.State {
		t.Errorf("Got unexpected result state -got %v +want %v", results[0].Result.State, request.Result.State)
	}
}

// TestProgressSink_ReportResultFailureCount makes sure the fake progress sink would set
// the terminate flag when maximum failures allowed has reached.
func TestProgressSink_ReportResultFailureCount(t *testing.T) {
	srv, addr, err := StartProgressSink(context.Background(), 1)
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer srv.Stop()

	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to establish connection to fake server: ", srv)
	}
	client := rtd.NewProgressSinkClient(conn)
	request := rtd.ReportResultRequest{
		Request: "FailededReq",
		Result: &resultspb.Result{
			State: resultspb.Result_FAILED,
			Errors: []*resultspb.Result_Error{
				{
					Source:   resultspb.Result_Error_TEST,
					Severity: resultspb.Result_Error_CRITICAL,
				},
			},
		},
	}

	rspn, err := client.ReportResult(context.Background(), &request)
	if err != nil {
		t.Fatal("Failed to call ReportResult: ", err)
	}
	if !rspn.Terminate {
		t.Errorf("rspn.Terminate is false; want true")
	}
}
