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
	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
)

// reportsServer implements the tast.internal.protocol.ReportsServer.
type reportsServer struct {
	srv      *grpc.Server
	listener net.Listener
}

var _ protocol.ReportsServer = (*reportsServer)(nil)

func (s reportsServer) LogStream(protocol.Reports_LogStreamServer) error {
	return nil
}

func (s reportsServer) ReportResult(ctx context.Context, req *protocol.ReportResultRequest) (*empty.Empty, error) {
	return nil, nil
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
func NewReportsServer(port int) (*reportsServer, error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	s := reportsServer{
		srv:      grpc.NewServer(),
		listener: l,
	}
	protocol.RegisterReportsServer(s.srv, &s)
	go s.srv.Serve(l)
	return &s, nil
}
