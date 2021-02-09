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

	resultspb "go.chromium.org/chromiumos/config/go/api/test/results/v2"
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

// FakeProgressSinkService is a fake service for testing testing Porgress Sink client.
type FakeProgressSinkService struct {
	rtd.UnimplementedProgressSinkServer
	Server *grpc.Server

	logMtx sync.Mutex
	// received log by ReportLog RPC
	log             map[nameAndRequest][]byte
	resultsMtx      sync.Mutex
	results         []*rtd.ReportResultRequest
	maxTestFailures int // If maxTestFailures > 0, terminate testing when the number failures reaches maximum allowed.
	failuresCount   int // The number of test failures so far.
}

// StartProgressSink starts a fake progress sink service.
func StartProgressSink(ctx context.Context, maxTestFailures int) (*FakeProgressSinkService, net.Addr, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to listen")
	}
	result := FakeProgressSinkService{
		log:             map[nameAndRequest][]byte{},
		maxTestFailures: maxTestFailures,
	}
	result.Server = result.Serve(ctx, l)
	return &result, l.Addr(), nil
}

// Serve starts FakeProgressSinkService.
func (s *FakeProgressSinkService) Serve(ctx context.Context, l net.Listener) *grpc.Server {
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
func (s *FakeProgressSinkService) Stop() {
	s.Server.Stop()
}

// ReceivedLog returns the bytes sent to the ReportLog fake API.
func (s *FakeProgressSinkService) ReceivedLog(name, request string) []byte {
	key := nameAndRequest{
		name:    name,
		request: request,
	}
	s.logMtx.Lock()
	defer s.logMtx.Unlock()
	return s.log[key]
}

// ReportLog implements rtd.ProgressSinkServer.ReportLog.
func (s *FakeProgressSinkService) ReportLog(stream rtd.ProgressSink_ReportLogServer) error {
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
func (s *FakeProgressSinkService) ReportResult(ctx context.Context, result *rtd.ReportResultRequest) (*rtd.ReportResultResponse, error) {
	s.resultsMtx.Lock()
	s.results = append(s.results, result)
	s.resultsMtx.Unlock()
	if result.Result.State == resultspb.Result_FAILED {
		for _, e := range result.Result.Errors {
			if e.Source == resultspb.Result_Error_TEST && e.Severity == resultspb.Result_Error_CRITICAL {
				s.failuresCount++
				break
			}
		}
	}
	return &rtd.ReportResultResponse{Terminate: s.maxTestFailures > 0 && s.failuresCount >= s.maxTestFailures}, nil
}

// Results returns a shallow copy of slice pointing to results sent to the ReportResult fake API.
func (s *FakeProgressSinkService) Results() []*rtd.ReportResultRequest {
	s.resultsMtx.Lock()
	defer s.resultsMtx.Unlock()
	return append([]*rtd.ReportResultRequest(nil), s.results...)
}
