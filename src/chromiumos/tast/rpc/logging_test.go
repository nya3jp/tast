// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestRemoteLogging(t *testing.T) {
	// Start remoteLoggingServer.
	sv := newRemoteLoggingServer()

	gs := grpc.NewServer()
	RegisterLoggingServer(gs, sv)

	lis, err := net.ListenTCP("tcp", nil)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	defer lis.Close()

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
	cl, err := newRemoteLoggingClient(context.Background(), conn, func(msg string) {
		logs <- msg
	})
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

func TestRemoteLoggingSeq(t *testing.T) {
	// Start remoteLoggingServer.
	sv := newRemoteLoggingServer()

	gs := grpc.NewServer()
	RegisterLoggingServer(gs, sv)

	lis, err := net.ListenTCP("tcp", nil)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	defer lis.Close()

	go gs.Serve(lis)
	defer gs.Stop()

	// Start remoteLoggingClient.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to dial: ", err)
	}
	defer conn.Close()

	cl, err := newRemoteLoggingClient(context.Background(), conn, func(msg string) {})
	if err != nil {
		t.Fatal("newRemoteLoggingClient failed: ", err)
	}
	defer cl.Close()

	// Initially, LastSeq should be 0.
	if lastSeq := sv.LastSeq(); lastSeq != 0 {
		t.Errorf("LastSeq() = %d; want %d", lastSeq, 0)
	}

	sv.Log("this is seq = 1")

	if lastSeq := sv.LastSeq(); lastSeq != 1 {
		t.Errorf("LastSeq() = %d; want %d", lastSeq, 1)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cl.WaitSeq(canceledCtx, 2); err == nil {
		t.Error("WaitSeq(2) unexpectedly succeeded")
	}

	sv.Log("this is seq = 2")

	shortCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cl.WaitSeq(shortCtx, 2); err != nil {
		t.Error("WaitSeq(2) failed: ", err)
	}
}
