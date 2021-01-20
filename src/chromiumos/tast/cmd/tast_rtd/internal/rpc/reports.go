// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides the RPC services by tast_rtd
package rpc

import (
	"context"
	"fmt"
	"net"

	"github.com/golang/protobuf/ptypes/empty"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast_rtd/internal/result"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// reportsServer implements the tast.internal.protocol.ReportsServer.
type reportsServer struct {
	srv              *grpc.Server           // RPC server to receive reports from tast.
	psClient         rtd.ProgressSinkClient // Progress Sink client to send reports.
	listener         net.Listener           // A listener for gRPC service.
	testsToRequests  map[string]string      // A mapping between test names and requests names.
	reportedRequests map[string]struct{}    // Requests that have received results.
}

var _ protocol.ReportsServer = (*reportsServer)(nil)

func (s reportsServer) LogStream(protocol.Reports_LogStreamServer) error {
	return nil
}

func (s reportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*empty.Empty, error) {
	requestName, ok := s.testsToRequests[req.Test]
	if !ok {
		return nil, errors.Errorf("cannot find request name for test %q", req.Test)
	}
	if err := result.SendTestResult(ctx, requestName, s.psClient, req); err != nil {
		return nil, errors.Wrap(err, "failed in ReportResult")
	}
	s.reportedRequests[requestName] = struct{}{}
	return &empty.Empty{}, nil
}

func (s reportsServer) Stop() {
	s.srv.Stop()
	s.listener.Close()
}

func (s reportsServer) Address() string {
	return s.listener.Addr().String()
}

// NewReportsServer starts a Reports gRPC service and returns a reportsServer object when success.
// The caller is responsible for calling Stop() method.
func NewReportsServer(port int, psClient rtd.ProgressSinkClient, testsToRequests map[string]string) (*reportsServer, error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	s := reportsServer{
		srv:              grpc.NewServer(),
		listener:         l,
		psClient:         psClient,
		testsToRequests:  testsToRequests,
		reportedRequests: make(map[string]struct{}),
	}
	protocol.RegisterReportsServer(s.srv, &s)
	go s.srv.Serve(l)
	return &s, nil
}
