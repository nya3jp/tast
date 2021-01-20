// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides the RPC services by tast_rtd
package rpc

import (
	"context"
	"fmt"
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

	mu               sync.Mutex          // A mutex to protect reportedRequests.
	reportedRequests map[string]struct{} // Requests that have received results.
}

var _ protocol.ReportsServer = (*ReportsServer)(nil)

// LogStream gets logs from tast and passed on to progress sink.
func (s *ReportsServer) LogStream(protocol.Reports_LogStreamServer) error {
	return nil
}

// ReportResult gets a report request from tast and passes on to progress sink.
func (s *ReportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*empty.Empty, error) {
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
	return &empty.Empty{}, nil
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
	s := ReportsServer{
		srv:              grpc.NewServer(),
		listenerAddr:     l.Addr(),
		psClient:         psClient,
		testsToRequests:  testsToRequests,
		reportedRequests: make(map[string]struct{}),
	}
	protocol.RegisterReportsServer(s.srv, &s)
	go s.srv.Serve(l)
	return &s, nil
}
