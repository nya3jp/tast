// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"net"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func TestTestServerListEntities(t *gotesting.T) {
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.Fixture{Name: "fixt1"}
	f2 := &testing.Fixture{Name: "fixt2"}

	reg := testing.NewRegistry()
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddFixture(f1)
	reg.AddFixture(f2)

	scfg := NewStaticConfig(reg, 0, Delegate{})

	// Set up a local gRPC server.
	srv := grpc.NewServer()
	protocol.RegisterTestServiceServer(srv, newTestServer(scfg))

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(lis)
	defer srv.Stop()

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cl := protocol.NewTestServiceClient(conn)

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
	if diff := cmp.Diff(got, want, cmpopts.SortSlices(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerListEntitiesTestSkips(t *gotesting.T) {
	features := &protocol.Features{
		CheckDeps: true,
		Software: &protocol.SoftwareFeatures{
			Available:   []string{"dep1"},
			Unavailable: []string{"dep2"},
		},
	}
	t1 := &testing.TestInstance{Name: "pkg.Test1", SoftwareDeps: []string{"dep1"}}
	t2 := &testing.TestInstance{Name: "pkg.Test2", SoftwareDeps: []string{"dep2"}}
	t3 := &testing.TestInstance{Name: "pkg.Test3", SoftwareDeps: []string{"dep3"}}

	reg := testing.NewRegistry()
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddTestInstance(t3)

	scfg := NewStaticConfig(reg, 0, Delegate{})

	// Set up a local gRPC server.
	srv := grpc.NewServer()
	protocol.RegisterTestServiceServer(srv, newTestServer(scfg))

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(lis)
	defer srv.Stop()

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cl := protocol.NewTestServiceClient(conn)

	// Call ListEntities.
	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{Features: features})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			// Test1 is not skipped.
			{Entity: t1.EntityProto()},
			// Test2 is skipped due to unavailable dep2.
			{Entity: t2.EntityProto(), Skip: &protocol.Skip{Reasons: []string{"missing SoftwareDeps: dep2"}}},
			// Test3 is not skipped due to a dependency check failure.
			// It fails later when we actually attempt to run it.
			{Entity: t3.EntityProto()},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, cmpopts.SortSlices(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}
