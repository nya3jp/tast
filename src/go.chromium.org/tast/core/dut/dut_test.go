// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dut_test

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/internal/sshtest"
)

var userKey, hostKey = sshtest.MustGenerateKeys()

// newDUTWithTunnel returns a DUT for a new SSH server, set up to connect
// through a new tunnel to it.
func newDUTWithTunnel(t *testing.T) (*dut.DUT, *sshtest.Tunnel) {
	t.Helper()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	t.Cleanup(func() { srv.Close() })
	keyFile, err := sshtest.WriteKey(userKey)
	if err != nil {
		t.Fatal("Failed writing key: ", err)
	}
	t.Cleanup(func() { os.Remove(keyFile) })
	tunnel, err := sshtest.NewTunnel(srv.Addr().String())
	if err != nil {
		t.Fatal("Failed starting tunnel: ", err)
	}
	t.Cleanup(func() { tunnel.Close() })

	d, err := dut.New(srv.Addr().String(), keyFile, "", "", nil, dut.WithTunnelAddress(tunnel.Addr().String()))
	if err != nil {
		t.Fatal("dut.New failed: ", err)
	}
	t.Cleanup(func() { d.Close(context.Background()) })
	return d, tunnel
}

func TestConnectTunnel(t *testing.T) {
	d, tunnel := newDUTWithTunnel(t)

	ctx := t.Context()
	for i := 0; i < 2; i++ {
		if err := d.Connect(ctx); err != nil {
			t.Fatalf("Connect #%d failed: %v", i+1, err)
		}
		// Only the first connection goes through the tunnel.
		if n := tunnel.Conns(); n != 1 {
			t.Fatalf("After Connect #%d, tunnel got %d connections; want 1", i+1, n)
		}
	}
}

func TestConnectTunnelFallback(t *testing.T) {
	d, tunnel := newDUTWithTunnel(t)
	// The closed tunnel refuses connections.
	tunnel.Close()

	if err := d.Connect(t.Context()); err != nil {
		t.Fatal("Connect failed: ", err)
	}
}

func TestConnectTunnelCanceled(t *testing.T) {
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	t.Cleanup(func() { srv.Close() })
	keyFile, err := sshtest.WriteKey(userKey)
	if err != nil {
		t.Fatal("Failed writing key: ", err)
	}
	t.Cleanup(func() { os.Remove(keyFile) })

	// A listener that never accepts connections stalls the tunnel handshake.
	stalled, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	t.Cleanup(func() { stalled.Close() })

	d, err := dut.New(srv.Addr().String(), keyFile, "", "", nil, dut.WithTunnelAddress(stalled.Addr().String()))
	if err != nil {
		t.Fatal("dut.New failed: ", err)
	}
	t.Cleanup(func() { d.Close(context.Background()) })

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err = d.Connect(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Connect error = %v; want %v", err, context.DeadlineExceeded)
	}
	if d.Conn() != nil {
		t.Error("DUT unexpectedly connected after context cancellation")
	}
}
