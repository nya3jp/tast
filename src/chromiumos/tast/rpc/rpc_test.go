// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"
	"reflect"
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

	rpcClient *Client
	stop      func() error // func to stop the gRPC server
}

// Close closes the gRPC connection and stops the gRPC server.
func (p *pingPair) Close(ctx context.Context) error {
	firstErr := p.rpcClient.Close(ctx)
	if err := p.stop(); firstErr == nil {
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
	stop := func() error {
		// Close the client pipes. This will let the gRPC server close the singleton
		// gRPC connection, which triggers the gRPC server to stop via pipeListener.
		cw.Close()
		cr.Close()
		return <-stopped
	}
	success := false
	defer func() {
		if !success {
			stop()
		}
	}()

	cl, err := newClient(ctx, cr, cw, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal("newClient failed: ", err)
	}

	success = true
	return &pingPair{
		Client:    NewPingClient(cl.Conn),
		rpcClient: cl,
		stop:      stop,
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

func TestRPCForwardTestContext(t *gotesting.T) {
	expectedDeps := []string{"chrome", "android"}

	ctx := context.Background()
	called := false
	pp := newPingPair(ctx, t, func(ctx context.Context) error {
		called = true
		if deps, ok := testing.ContextSoftwareDeps(ctx); !ok {
			return errors.New("SoftwareDeps unavailable")
		} else if !reflect.DeepEqual(deps, expectedDeps) {
			return errors.Errorf("SoftwareDeps mismatch: got %v, want %v", deps, expectedDeps)
		}
		return nil
	})
	defer pp.Close(ctx)

	if _, err := pp.Client.Ping(ctx, &PingRequest{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context without TestContext")
	}

	callCtx := testing.WithTestContext(ctx, &testing.TestContext{
		ServiceDeps:  []string{pingServiceName},
		SoftwareDeps: expectedDeps,
	})
	if _, err := pp.Client.Ping(callCtx, &PingRequest{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
}
