// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakerts provides a fake implementation of the RTS service.
package fakerts

import (
	"context"
	"io"
	"net"

	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// identifier for ProgressSinkServirce.ReportLog requests
type nameAndRequest struct {
	name    string
	request string
}

type fakeProgressSinkService struct {
	rtd.UnimplementedProgressSinkServer
	Server *grpc.Server

	// received log by ReportLog RPC
	log     map[nameAndRequest][]byte
	results []*rtd.ReportResultRequest
}

// StartProgressSink starts a fake progress sink service.
func StartProgressSink(ctx context.Context) (*fakeProgressSinkService, net.Addr, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to listen")
	}
	result := fakeProgressSinkService{
		log: map[nameAndRequest][]byte{},
	}
	result.Server = result.Serve(ctx, l)
	return &result, l.Addr(), nil
}

func (s *fakeProgressSinkService) Serve(ctx context.Context, l net.Listener) *grpc.Server {
	server := grpc.NewServer()
	rtd.RegisterProgressSinkServer(server, s)
	// Start the server in a background thread, since the Serve() call blocks.
	go func() {
		if err := server.Serve(l); err != nil {
			testing.ContextLog(ctx, "ProgressSinkService failed: ", err)
		}
	}()
	return server
}

// Stop stops the fake ProgressSinkeService.
func (s *fakeProgressSinkService) Stop() {
	s.Server.Stop()
}

// ReceivedLog returns the bytes sent to the ReportLog fake API.
func (s *fakeProgressSinkService) ReceivedLog(name, request string) []byte {
	key := nameAndRequest{
		name:    name,
		request: request,
	}
	return s.log[key]
}

func (s *fakeProgressSinkService) ReportLog(stream rtd.ProgressSink_ReportLogServer) error {
	for {
		data, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&rtd.ReportLogResponse{})
		}
		key := nameAndRequest{
			name:    data.Name,
			request: data.Request,
		}
		s.log[key] = append(s.log[key], data.Data...)
	}
}

func (s *fakeProgressSinkService) ReportResult(ctx context.Context, result *rtd.ReportResultRequest) (*rtd.ReportResultResponse, error) {
	s.results = append(s.results, result)
	return &rtd.ReportResultResponse{}, nil
}

func (s *fakeProgressSinkService) Results() []*rtd.ReportResultRequest {
	return s.results
}
