// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"net"
	gotesting "testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/testing"
)

func TestManagement(t *gotesting.T) {
	// Start managementServer.
	md := &testing.ServiceManagementData{}
	sv := newManagementServer(md)

	gs := grpc.NewServer()
	RegisterManagementServer(gs, sv)

	lis, err := net.ListenTCP("tcp", nil)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go gs.Serve(lis)
	defer gs.Stop()

	// Start managementClient.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to dial: ", err)
	}
	defer conn.Close()

	cl := NewManagementClient(conn)

	vars := map[string]string{
		"var1": "value1",
		"var2": "value2",
	}

	// On the server side, test vars should be empty before client sets them.
	for key := range vars {
		if v, ok := md.Var(key); ok {
			t.Errorf("Test variable %q - got value %q; want non-existent", key, v)
		}
	}

	testVars := &TestVars{
		Var: vars,
	}
	ctx := context.Background()
	if _, err := cl.SetTestVars(ctx, testVars); err != nil {
		t.Fatal("Failed to SetTestVars: ", err)
	}

	// test vars should be set on the server side.
	for key, value := range vars {
		if v, ok := md.Var(key); !ok || v != value {
			t.Errorf("Got test variable %q value %q; want %q", key, v, value)
		}
	}
}
