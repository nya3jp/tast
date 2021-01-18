// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakereports provides a fake implementation of Reports service for unit testing.
package fakereports

import (
	"context"
	"net"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/internal/protocol"
)

type fakeReportsServer struct{}

var _ protocol.ReportsServer = &fakeReportsServer{}

// Start starts a gRPC server serving ReportsServer in the background for tests.
// Callers are responsible for stopping the server by stopFunc().
func Start(t *testing.T) (stopFunc func(), addr string) {
	s := &fakeReportsServer{}
	srv := grpc.NewServer()
	protocol.RegisterReportsServer(srv, s)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	go srv.Serve(lis)
	return srv.Stop, lis.Addr().String()
}

func (*fakeReportsServer) LogStream(srv protocol.Reports_LogStreamServer) error {
	// TODO(crbug.com/1166951): Implement for unit tests.
	return status.Errorf(codes.Unimplemented, "method LogStream not implemented")
}

func (*fakeReportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*empty.Empty, error) {
	// TODO(crbug.com/1166955): Implement for unit tests.
	return nil, status.Errorf(codes.Unimplemented, "method ReportResult not implemented")
}
