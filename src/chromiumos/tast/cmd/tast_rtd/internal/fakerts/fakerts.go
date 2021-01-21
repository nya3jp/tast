// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakerts provides a fake implementation of the RTS service.
package fakerts

import (
	"context"
	"io"
	"net"
	"sync"

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

	logMtx sync.Mutex
	// received log by ReportLog RPC
	log        map[nameAndRequest][]byte
	resultsMtx sync.Mutex
	results    []*rtd.ReportResultRequest
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
	s.logMtx.Lock()
	defer s.logMtx.Unlock()
	return s.log[key]
}

// ReportLog implements rtd.ProgressSinkServer.ReportLog.
func (s *fakeProgressSinkService) ReportLog(stream rtd.ProgressSink_ReportLogServer) error {
	for {
		data, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&rtd.ReportLogResponse{})
		}
		if err != nil {
			return err
		}
		key := nameAndRequest{
			name:    data.Name,
			request: data.Request,
		}
		s.logMtx.Lock()
		s.log[key] = append(s.log[key], data.Data...)
		s.logMtx.Unlock()
	}
}

// ReportResult implements rtd.ProgressSinkServer.ReportResult.
func (s *fakeProgressSinkService) ReportResult(ctx context.Context, result *rtd.ReportResultRequest) (*rtd.ReportResultResponse, error) {
	s.resultsMtx.Lock()
	s.results = append(s.results, result)
	s.resultsMtx.Unlock()
	return &rtd.ReportResultResponse{}, nil
}

// Result returns a shallow copy of slice pointing to results sent to the ReportResult fake API.
func (s *fakeProgressSinkService) Results() []*rtd.ReportResultRequest {
	s.resultsMtx.Lock()
	defer s.resultsMtx.Unlock()
	return append([]*rtd.ReportResultRequest(nil), s.results...)
}
