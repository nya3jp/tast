// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
)

func TestReportsServer_LogStream(t *testing.T) {
	srv, err := NewReportsServer(0)
	if err != nil {
		t.Fatalf("Failed to start Reports server: %v", err)
	}
	addr := srv.Address()
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Test that the server is started and reachable by calling a method.
	// TODO(crbug.com/1166942): Test with actual usage of LogStream.
	cl := protocol.NewReportsClient(conn)
	if _, err := cl.LogStream(context.Background()); err != nil {
		t.Fatalf("Failed at LogStream: %v", err)
	}
}
