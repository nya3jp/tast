// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakereports provides a fake implementation of Reports service for unit testing.
package fakereports

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/internal/protocol"
)

type fakeReportsServer struct {
	logData map[string][]byte
}

var _ protocol.ReportsServer = &fakeReportsServer{}

// Start starts a gRPC server serving ReportsServer in the background for tests.
// Callers are responsible for stopping the server by stopFunc().
func Start(t *testing.T) (server *fakeReportsServer, stopFunc func(), addr string) {
	s := &fakeReportsServer{}
	srv := grpc.NewServer()
	protocol.RegisterReportsServer(srv, s)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	s.logData = make(map[string][]byte)
	go srv.Serve(lis)
	return s, srv.Stop, lis.Addr().String()
}

func (s *fakeReportsServer) LogStream(stream protocol.Reports_LogStreamServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&empty.Empty{})
		}
		if err != nil {
			return err
		}
		test := req.Test
		s.logData[test] = append(s.logData[test], req.Data...)
	}
}

func (s *fakeReportsServer) GetLog(test string) []byte {
	return s.logData[test]
}

func (*fakeReportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*empty.Empty, error) {
	// TODO(crbug.com/1166955): Implement for unit tests.
	return nil, status.Errorf(codes.Unimplemented, "method ReportResult not implemented")
}
