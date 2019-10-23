// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"
	gotesting "testing"

	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

const pingServiceName = "tast.core.Ping"

// pingServer is an implementation of the Ping gRPC service.
type pingServer struct {
	// onPing is called when Ping is called by gRPC clients.
	onPing func(context.Context) error
}

func (s *pingServer) Ping(ctx context.Context, _ *PingRequest) (*PingResponse, error) {
	if err := s.onPing(ctx); err != nil {
		return nil, err
	}
	return &PingResponse{}, nil
}

var _ PingServer = (*pingServer)(nil)

// pingPair manages a local client/server pair of the Ping gRPC service.
type pingPair struct {
	Client PingClient
	// The server is missing here; it is implicitly owned by the background
	// goroutine that calls RunServer.

	rpcClient  *Client      // underlying gRPC connection of Client
	stopServer func() error // func to stop the gRPC server
}

// Close closes the gRPC connection and stops the gRPC server.
func (p *pingPair) Close(ctx context.Context) error {
	firstErr := p.rpcClient.Close(ctx)
	if err := p.stopServer(); firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// newPingPair starts a local client/server pair of the Ping gRPC service.
// onPing is called when Ping gRPC method is called on the server.
//
// It panics if it fails to start a local client/server pair. Returned pingPair
// should be closed with pingPair.Close after its use.
func newPingPair(ctx context.Context, t *gotesting.T, onPing func(context.Context) error) *pingPair {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	svc := &testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			RegisterPingServer(srv, &pingServer{onPing})
		},
	}

	stopped := make(chan error, 1)
	go func() {
		stopped <- RunServer(sr, sw, []*testing.Service{svc})
	}()
	stopServer := func() error {
		// Close the client pipes. This will let the gRPC server close the singleton
		// gRPC connection, which triggers the gRPC server to stop via pipeListener.
		cw.Close()
		cr.Close()
		return <-stopped
	}
	success := false
	defer func() {
		if !success {
			stopServer() // no error check; test has already failed
		}
	}()

	cl, err := newClient(ctx, cr, cw, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal("newClient failed: ", err)
	}

	success = true
	return &pingPair{
		Client:     NewPingClient(cl.Conn),
		rpcClient:  cl,
		stopServer: stopServer,
	}
}

func TestRPCSuccess(t *gotesting.T) {
	ctx := context.Background()
	called := false
	pp := newPingPair(ctx, t, func(context.Context) error {
		called = true
		return nil
	})
	defer pp.Close(ctx)

	callCtx := testing.WithTestContext(ctx, &testing.TestContext{
		ServiceDeps: []string{pingServiceName},
	})
	if _, err := pp.Client.Ping(callCtx, &PingRequest{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCFailure(t *gotesting.T) {
	ctx := context.Background()
	called := false
	pp := newPingPair(ctx, t, func(context.Context) error {
		called = true
		return errors.New("failure")
	})
	defer pp.Close(ctx)

	callCtx := testing.WithTestContext(ctx, &testing.TestContext{
		ServiceDeps: []string{pingServiceName},
	})
	if _, err := pp.Client.Ping(callCtx, &PingRequest{}); err == nil {
		t.Error("Ping unexpectedly succeeded")
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCNoTestContext(t *gotesting.T) {
	ctx := context.Background()
	called := false
	pp := newPingPair(ctx, t, func(context.Context) error {
		called = true
		return nil
	})
	defer pp.Close(ctx)

	if _, err := pp.Client.Ping(ctx, &PingRequest{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context missing TestContext")
	}
	if called {
		t.Error("onPing unexpectedly called")
	}
}

func TestRPCRejectUndeclaredServices(t *gotesting.T) {
	ctx := context.Background()
	pp := newPingPair(ctx, t, func(context.Context) error { return nil })
	defer pp.Close(ctx)

	callCtx := testing.WithTestContext(ctx, &testing.TestContext{
		ServiceDeps: []string{"foo.Bar"},
	})
	if _, err := pp.Client.Ping(callCtx, &PingRequest{}); err == nil {
		t.Error("Ping unexpectedly succeeded despite undeclared service")
	}
}
