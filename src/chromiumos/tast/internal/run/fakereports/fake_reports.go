// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakereports provides a fake implementation of Reports service for unit testing.
package fakereports

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/framework/protocol"
)

// Server provides a fake resports server to test reports client implementation.
type Server struct {
	mtx             sync.Mutex
	logData         map[logKey][]byte
	results         []*protocol.ReportResultRequest
	maxTestFailures int // maxTestFailures specifies maximum number of test allowed. Zero means unlimited.
	failuresCount   int // The number of test failures so far.
}

type logKey struct {
	test    string
	logPath string
}

var _ protocol.ReportsServer = &Server{}

// Start starts a gRPC server serving ReportsServer in the background for tests.
// Callers are responsible for stopping the server by stopFunc().
func Start(t *testing.T, maxTestFailures int) (server *Server, stopFunc func(), addr string) {
	s := &Server{}
	srv := grpc.NewServer()
	protocol.RegisterReportsServer(srv, s)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	s.logData = make(map[logKey][]byte)
	s.maxTestFailures = maxTestFailures
	go srv.Serve(lis)
	return s, srv.Stop, lis.Addr().String()
}

// LogStream provides a means for reports clients to test LogStream requests.
func (s *Server) LogStream(stream protocol.Reports_LogStreamServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&empty.Empty{})
		}
		if err != nil {
			return err
		}
		test := req.Test
		key := logKey{
			test:    test,
			logPath: req.LogPath,
		}
		s.mtx.Lock()
		s.logData[key] = append(s.logData[key], req.Data...)
		s.mtx.Unlock()
	}
}

// GetLog returns logs for a particular test.
func (s *Server) GetLog(test, logPath string) []byte {
	key := logKey{
		test:    test,
		logPath: logPath,
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.logData[key]
}

// ReportResult provides a means for reports clients to test ReportResult requests.
func (s *Server) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*protocol.ReportResultResponse, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.results = append(s.results, req)
	if len(req.Errors) > 0 {
		s.failuresCount++
	}
	return &protocol.ReportResultResponse{Terminate: s.maxTestFailures > 0 && s.failuresCount >= s.maxTestFailures}, nil
}

// Results returns all results have been received for this server.
func (s *Server) Results() []*protocol.ReportResultRequest {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.results
}
