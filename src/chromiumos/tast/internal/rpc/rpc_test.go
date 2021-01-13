// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	gotesting "testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/testutil"
)

const pingUserServiceName = "tast.coretest.PingUser"

// pingUserServer is an implementation of the Ping gRPC service.
type pingUserServer struct {
	s *testing.ServiceState
	// onPing is called when Ping is called by gRPC clients.
	onPing func(context.Context, *testing.ServiceState) error
}

func (s *pingUserServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := s.onPing(ctx, s.s); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

type pingCoreServer struct{}

func (s *pingCoreServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// pingPair manages a local client/server pair of the Ping gRPC service.
type pingPair struct {
	UserClient protocol.PingUserClient
	CoreClient protocol.PingCoreClient
	// The server is missing here; it is implicitly owned by the background
	// goroutine that calls RunServer.

	rpcClient  *Client      // underlying gRPC connection
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

// newPingService defines a new Ping service.
// onPing is called when Ping gRPC method is called on the server.
func newPingService(onPing func(context.Context, *testing.ServiceState) error) *testing.Service {
	return &testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			protocol.RegisterPingUserServer(srv, &pingUserServer{s, onPing})
		},
	}
}

// newPingPair starts a local client/server pair of the Ping gRPC service.
//
// It panics if it fails to start a local client/server pair. Returned pingPair
// should be closed with pingPair.Close after its use.
func newPingPair(ctx context.Context, t *gotesting.T, req *protocol.HandshakeRequest, pingSvc *testing.Service) *pingPair {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	stopped := make(chan error, 1)
	go func() {
		stopped <- RunServer(sr, sw, []*testing.Service{pingSvc}, func(srv *grpc.Server) {
			protocol.RegisterPingCoreServer(srv, &pingCoreServer{})
		})
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

	cl, err := newClient(ctx, cr, cw, req, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal("newClient failed: ", err)
	}

	success = true
	return &pingPair{
		UserClient: protocol.NewPingUserClient(cl.Conn),
		CoreClient: protocol.NewPingCoreClient(cl.Conn),
		rpcClient:  cl,
		stopServer: stopServer,
	}
}

func TestRPCSuccess(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCFailure(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return errors.New("failure")
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded")
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCNotRequested(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{} // user-defined gRPC services not requested

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded")
	}
	if called {
		t.Error("onPing unexpectedly called")
	}
}

func TestRPCNoCurrentEntity(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	if _, err := pp.UserClient.Ping(context.Background(), &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context missing CurrentEntity")
	}
	if called {
		t.Error("onPing unexpectedly called")
	}
}

func TestRPCRejectUndeclaredServices(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}
	svc := newPingService(func(context.Context, *testing.ServiceState) error { return nil })
	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{"foo.Bar"},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded despite undeclared service")
	}
}

func TestRPCForwardCurrentEntity(t *gotesting.T) {
	expectedDeps := []string{"chrome", "android_p"}

	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	var deps []string
	var depsOK bool
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		deps, depsOK = testcontext.SoftwareDeps(ctx)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	if _, err := pp.UserClient.Ping(ctx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context without CurrentEntity")
	}

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps:     []string{pingUserServiceName},
		HasSoftwareDeps: true,
		SoftwareDeps:    expectedDeps,
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	} else if !depsOK {
		t.Error("SoftwareDeps unavailable")
	} else if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("SoftwareDeps mismatch: got %v, want %v", deps, expectedDeps)
	}
}

func TestRPCForwardLogs(t *gotesting.T) {
	const exp = "hello"

	logs := make(chan string, 1)
	ctx := context.Background()
	ctx = testcontext.WithLogger(ctx, func(msg string) { logs <- msg })
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		testcontext.Log(ctx, exp)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	if msg := <-logs; msg != exp {
		t.Errorf("Got log %q; want %q", msg, exp)
	}
}

func TestRPCForwardTiming(t *gotesting.T) {
	const stageName = "hello"

	ctx := context.Background()
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	log := timing.NewLog()
	ctx = timing.NewContext(ctx, log)
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		_, st := timing.Start(ctx, stageName)
		st.End()
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	if len(log.Root.Children) != 1 || log.Root.Children[0].Name != stageName {
		b, err := json.Marshal(log)
		if err != nil {
			t.Fatal("Failed to marshal timing JSON: ", err)
		}
		t.Errorf("Unexpected timing log: got %s, want a single %q entry", string(b), stageName)
	}
}

func TestRPCPullOutDir(t *gotesting.T) {
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	want := map[string]string{
		"a.txt":     "abc",
		"dir/b.txt": "def",
	}

	ctx := context.Background()
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		od, ok := testcontext.OutDir(ctx)
		if !ok {
			return errors.New("OutDir unavailable")
		}
		if od == outDir {
			return errors.Errorf("OutDir given to service must not be that on the host: %s", od)
		}
		return testutil.WriteFiles(od, want)
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
		OutDir:      outDir,
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}

	got, err := testutil.ReadFiles(outDir)
	if err != nil {
		t.Fatal("Failed to read output dir: ", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Directory contents mismatch (-got +want):\n%s", diff)
	}
}

func TestRPCSetVars(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	key := "var1"
	exp := "value1"
	req := &protocol.HandshakeRequest{
		NeedUserServices: true,
		UserServiceInitParams: &protocol.UserServiceInitParams{
			Vars: map[string]string{key: exp},
		},
	}

	called := false
	var value string
	ok := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		value, ok = s.Var(key)
		return nil
	})
	// Set service vars in service definition.
	svc.Vars = []string{key}

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
	if !ok || value != exp {
		t.Errorf("Runtime var not set for key %q: got ok %t and value %q, want %q", key, ok, value, exp)
	}
}

func TestRPCServiceScopedContext(t *gotesting.T) {
	const exp = "hello"

	logs := make(chan string, 1)
	ctx := context.Background()
	ctx = testcontext.WithLogger(ctx, func(msg string) { logs <- msg })
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		testcontext.Log(s.ServiceContext(), exp)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	if msg := <-logs; msg != exp {
		t.Errorf("Got log %q; want %q", msg, exp)
	}
}

func TestRPCExtraCoreServices(t *gotesting.T) {
	ctx := context.Background()
	req := &protocol.HandshakeRequest{NeedUserServices: false}
	svc := newPingService(nil)

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close(ctx)

	if _, err := pp.CoreClient.Ping(ctx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
}
