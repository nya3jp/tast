// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"
	"io/ioutil"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/internal/bundle/fakebundle"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
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
		Run([]string{"-rpc"}, sr, sw, ioutil.Discard, &jsonprotocol.RunnerArgs{}, &StaticConfig{})
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

func TestTestServer(t *gotesting.T) {
	// Create two fake bundles.
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.Fixture{Name: "fixt1"}
	f2 := &testing.Fixture{Name: "fixt2"}

	reg1 := testing.NewRegistry()
	reg2 := testing.NewRegistry()
	reg1.AddTestInstance(t1)
	reg2.AddTestInstance(t2)
	reg1.AddFixture(f1)
	reg2.AddFixture(f2)

	bundleGlob := fakebundle.Install(t, map[string]*testing.Registry{"a": reg1, "b": reg2})

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
	if diff := cmp.Diff(got, want, cmpopts.SortSlices(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}
