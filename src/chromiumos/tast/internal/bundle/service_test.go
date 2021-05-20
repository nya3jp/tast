// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"io/ioutil"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// startTestServer starts an in-process gRPC server and returns a connection as
// TestServiceClient. On completion of the current test, resources are released
// automatically.
func startTestServer(t *gotesting.T, scfg *StaticConfig) protocol.TestServiceClient {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	t.Cleanup(func() {
		cw.Close()
		cr.Close()
	})

	go run(context.Background(), []string{"-rpc"}, sr, sw, ioutil.Discard, scfg)

	conn, err := rpc.NewClient(context.Background(), cr, cw, &protocol.HandshakeRequest{})
	if err != nil {
		t.Fatalf("Failed to connect to in-process gRPC server: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	return protocol.NewTestServiceClient(conn.Conn())
}

func TestTestServerListEntities(t *gotesting.T) {
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.FixtureInstance{Name: "fixt1"}
	f2 := &testing.FixtureInstance{Name: "fixt2"}

	reg := testing.NewRegistry()
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddFixtureInstance(f1)
	reg.AddFixtureInstance(f2)

	scfg := NewStaticConfig(reg, 0, Delegate{})

	cl := startTestServer(t, scfg)

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
		Dut: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   []string{"dep1"},
				Unavailable: []string{"dep2"},
			},
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

	cl := startTestServer(t, scfg)

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
