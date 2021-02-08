// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides the RPC services by tast_rtd
package rpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast_rtd/internal/result"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// ReportsServer implements the tast.internal.protocol.ReportsServer.
type ReportsServer struct {
	srv             *grpc.Server           // RPC server to receive reports from tast.
	psClient        rtd.ProgressSinkClient // Progress Sink client to send reports.
	listenerAddr    net.Addr               // The address for the listener for gRPC service.
	testsToRequests map[string]string      // A mapping between test names and requests names.
	reportLogStream rtd.ProgressSink_ReportLogClient

	mu               sync.Mutex          // A mutex to protect reportedRequests.
	reportedRequests map[string]struct{} // Requests that have received results.
}

var _ protocol.ReportsServer = (*ReportsServer)(nil)

// LogStream gets logs from tast and passes on to progress sink server.
func (s *ReportsServer) LogStream(stream protocol.Reports_LogStreamServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&empty.Empty{})
		}
		if err != nil {
			return err
		}
		request, ok := s.testsToRequests[in.Test]
		if !ok {
			return errors.Errorf("cannot find request name for test %q", in.Test)
		}
		req := rtd.ReportLogRequest{
			Name:    in.LogPath,
			Request: request,
			Data:    in.Data,
		}
		if err := s.reportLogStream.Send(&req); err != nil {
			return errors.Wrap(err, "failed to send to ReportLog stream")
		}
	}
}

// ReportResult gets a report request from tast and passes on to progress sink.
func (s *ReportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*protocol.ReportResultResponse, error) {
	requestName, ok := s.testsToRequests[req.Test]
	if !ok {
		return nil, errors.Errorf("cannot find request name for test %q", req.Test)
	}
	if err := result.SendTestResult(ctx, requestName, s.psClient, req); err != nil {
		return nil, errors.Wrap(err, "failed in ReportResult")
	}
	s.mu.Lock()
	s.reportedRequests[requestName] = struct{}{}
	s.mu.Unlock()
	return &protocol.ReportResultResponse{}, nil
}

// SendMissingTestsReports sends reports to progress sink on all the tests that are not run by tast.
func (s *ReportsServer) SendMissingTestsReports(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, req := range s.testsToRequests {
		if _, ok := s.reportedRequests[req]; ok {
			continue
		}
		if err := result.SendReqToProgressSink(ctx, s.psClient, result.MissingTestResult(req)); err != nil {
			return errors.Wrapf(err, "failed in sending missing test report for request %q", req)
		}
	}
	return nil
}

// Stop stops the ReportsServer.
func (s *ReportsServer) Stop() {
	s.reportLogStream.CloseAndRecv()
	s.srv.Stop()
}

// Address returns the network address of the ReportsServer.
func (s *ReportsServer) Address() string {
	return s.listenerAddr.String()
}

// NewReportsServer starts a Reports gRPC service and returns a ReportsServer object when success.
// The caller is responsible for calling Stop() method.
func NewReportsServer(port int, psClient rtd.ProgressSinkClient, testsToRequests map[string]string) (*ReportsServer, error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	stream, err := psClient.ReportLog(context.Background())
	if err != nil {
		l.Close()
		return nil, err
	}
	s := ReportsServer{
		srv:              grpc.NewServer(),
		listenerAddr:     l.Addr(),
		psClient:         psClient,
		testsToRequests:  testsToRequests,
		reportedRequests: make(map[string]struct{}),
		reportLogStream:  stream,
	}

	protocol.RegisterReportsServer(s.srv, &s)
	go s.srv.Serve(l)
	return &s, nil
}
