// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"io/ioutil"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/bundle/fakebundle"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/protocol/protocoltest"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// startTestServer starts an in-process gRPC server and returns a connection as
// TestServiceClient. On completion of the current test, resources are released
// automatically.
func startTestServer(t *gotesting.T, params *protocol.RunnerInitParams) protocol.TestServiceClient {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run([]string{"-rpc"}, sr, sw, ioutil.Discard, &StaticConfig{})
	}()
	t.Cleanup(func() {
		cw.Close()
		cr.Close()
		<-done
	})

	conn, err := rpc.NewClient(context.Background(), cr, cw, &protocol.HandshakeRequest{RunnerInitParams: params})
	if err != nil {
		t.Fatalf("Failed to connect to in-process gRPC server: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	return protocol.NewTestServiceClient(conn.Conn())
}

func TestTestServerListEntities(t *gotesting.T) {
	// Create two fake bundles.
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.FixtureInstance{Name: "fixt1"}
	f2 := &testing.FixtureInstance{Name: "fixt2"}

	reg1 := testing.NewRegistry("a")
	reg2 := testing.NewRegistry("b")
	reg1.AddTestInstance(t1)
	reg2.AddTestInstance(t2)
	reg1.AddFixtureInstance(f1)
	reg2.AddFixtureInstance(f2)

	bundleGlob := fakebundle.Install(t, reg1, reg2)

	cl := startTestServer(t, &protocol.RunnerInitParams{BundleGlob: bundleGlob})

	// Call ListEntities.
	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			{Entity: f1.EntityProto()},
			{Entity: f2.EntityProto()},
			{Entity: t1.EntityProto()},
			{Entity: t2.EntityProto()},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerRunTests(t *gotesting.T) {
	// Create two fake bundles.
	test1 := &testing.TestInstance{
		Name:    "pkg.Test1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}
	test2 := &testing.TestInstance{
		Name: "pkg.Test2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Test2 failed")
		},
		Timeout: time.Minute,
	}
	test3 := &testing.TestInstance{
		Name:    "pkg.Test3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}

	reg1 := testing.NewRegistry("bundle1")
	reg1.AddTestInstance(test1)
	reg1.AddTestInstance(test2)
	reg2 := testing.NewRegistry("bundle2")
	reg2.AddTestInstance(test3)

	bundleGlob := fakebundle.Install(t, reg1, reg2)

	cl := startTestServer(t, &protocol.RunnerInitParams{BundleGlob: bundleGlob})

	// Call RunTests.
	events, err := protocoltest.RunTestsForEvents(context.Background(), cl, &protocol.RunConfig{})
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: test1.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test1.Name},
		&protocol.EntityStartEvent{Entity: test2.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: test2.Name, Error: &protocol.Error{Reason: "Test2 failed"}},
		&protocol.EntityEndEvent{EntityName: test2.Name},
		&protocol.EntityStartEvent{Entity: test3.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test3.Name},
	}
	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}
