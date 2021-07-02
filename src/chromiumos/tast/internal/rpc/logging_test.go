// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"net"
	"strconv"
	"testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

func TestRemoteLogging(t *testing.T) {
	// Start remoteLoggingServer.
	sv := newRemoteLoggingServer()

	gs := grpc.NewServer()
	protocol.RegisterLoggingServer(gs, sv)

	lis, err := net.ListenTCP("tcp", nil)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go gs.Serve(lis)
	defer gs.Stop()

	// Start remoteLoggingClient.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to dial: ", err)
	}
	defer conn.Close()

	sv.Log("foo") // logs before ReadLogs should be discarded

	logs := make(chan string)
	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) { logs <- msg }))
	ctx := logging.AttachLogger(context.Background(), logger)
	cl, err := newRemoteLoggingClient(ctx, conn)
	if err != nil {
		t.Fatal("newRemoteLoggingClient failed: ", err)
	}
	// Do not defer cl.Close() here since we call it below. There is a risk of
	// leaking cl, but it's okay in unit tests.

	// Logs should be delivered in order.
	const n = 100
	for i := 0; i < n; i++ {
		sv.Log(strconv.Itoa(i)) // should not block
	}

	for i := 0; i < n; i++ {
		got := <-logs
		want := strconv.Itoa(i)
		if got != want {
			t.Errorf("Unexpected log entry at position %d: got %q, want %q", i, got, want)
		}
	}

	if err := cl.Close(); err != nil {
		t.Fatal("remoteLoggingClient.Close failed: ", err)
	}

	sv.Log("foo") // logs after ReadLogs should be discarded
}
