// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc"

	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// createFakeBundles creates fake test bundles corresponding to regs, and
// returns a file path glob that matches file paths.
func createFakeBundles(t *gotesting.T, regs ...*testing.Registry) (bundleGlob string) {
	t.Helper()

	dir, err := ioutil.TempDir("", "tast-fakebundles.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	for i, reg := range regs {
		name := fmt.Sprintf("bundle%d", i)
		reg := reg
		lo, err := fakeexec.CreateLoopback(filepath.Join(dir, name), func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
			return bundle.Local(args[1:], stdin, stdout, stderr, reg, bundle.Delegate{})
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lo.Close() })
	}
	return filepath.Join(dir, "*")
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

	bundleGlob := createFakeBundles(t, reg1, reg2)

	// Set up a local gRPC server.
	srv := grpc.NewServer()
	protocol.RegisterTestServiceServer(srv, newTestServer(bundleGlob))

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
