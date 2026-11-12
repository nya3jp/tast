// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"os"
	"testing"

	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh"
)

// startTestServer starts an SSH server running commands locally, and returns
// a Config and a target to connect to it.
func startTestServer(t *testing.T) (*Config, string) {
	t.Helper()
	userKey, hostKey := sshtest.MustGenerateKeys()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(req *sshtest.ExecReq) {
		req.Start(true)
		req.End(req.RunRealCmd())
	})
	if err != nil {
		t.Fatal("NewSSHServer failed: ", err)
	}
	t.Cleanup(func() { srv.Close() })

	keyFile, err := sshtest.WriteKey(userKey)
	if err != nil {
		t.Fatal("WriteKey failed: ", err)
	}
	t.Cleanup(func() { os.Remove(keyFile) })

	cfg := &Config{
		SSHConfig:     &protocol.SSHConfig{KeyFile: keyFile},
		ServiceConfig: &ServiceConfig{},
	}
	return cfg, srv.Addr().String()
}

// newTestConnCache returns a ConnCache connected to a new test server.
func newTestConnCache(ctx context.Context, t *testing.T) *ConnCache {
	t.Helper()
	cfg, target := startTestServer(t)
	cc, err := NewConnCache(ctx, cfg, target, "", "", true)
	if err != nil {
		t.Fatal("NewConnCache failed: ", err)
	}
	return cc
}

// runTrue runs the true command over hst, which fails if hst is closed.
func runTrue(ctx context.Context, hst *ssh.Conn) error {
	return hst.CommandContext(ctx, "true").Run()
}

func TestConnCacheDuplicate(t *testing.T) {
	ctx := context.Background()
	cc := newTestConnCache(ctx, t)
	hst := cc.Conn().SSHConn()

	dup, err := cc.Duplicate(ctx)
	if err != nil {
		t.Fatal("Duplicate failed: ", err)
	}
	if dup.Conn().SSHConn() != hst {
		t.Error("Duplicate does not share the connection")
	}
	if dup.InitBootID() != cc.InitBootID() {
		t.Errorf("InitBootID = %q; want %q", dup.InitBootID(), cc.InitBootID())
	}

	if err := cc.Close(ctx); err != nil {
		t.Error("cc.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err != nil {
		t.Error("Connection is unusable after closing cc: ", err)
	}
	if err := dup.Close(ctx); err != nil {
		t.Error("dup.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err == nil {
		t.Error("Connection is still open after closing cc and dup")
	}
}

func TestConnCacheDuplicateMultiple(t *testing.T) {
	ctx := context.Background()
	cc := newTestConnCache(ctx, t)
	hst := cc.Conn().SSHConn()

	dup1, err := cc.Duplicate(ctx)
	if err != nil {
		t.Fatal("cc.Duplicate failed: ", err)
	}
	dup2, err := dup1.Duplicate(ctx)
	if err != nil {
		t.Fatal("dup1.Duplicate failed: ", err)
	}

	// The connection stays open until the last ConnCache is closed, in any
	// order.
	if err := dup1.Close(ctx); err != nil {
		t.Error("dup1.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err != nil {
		t.Error("Connection is unusable after closing dup1: ", err)
	}
	if err := cc.Close(ctx); err != nil {
		t.Error("cc.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err != nil {
		t.Error("Connection is unusable after closing dup1 and cc: ", err)
	}
	if err := dup2.Close(ctx); err != nil {
		t.Error("dup2.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err == nil {
		t.Error("Connection is still open after closing all ConnCaches")
	}
}

func TestConnCacheDuplicateFailure(t *testing.T) {
	ctx := context.Background()
	cc := newTestConnCache(ctx, t)
	hst := cc.Conn().SSHConn()

	// Reading the boot ID fails with a canceled context.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := cc.Duplicate(canceledCtx); err == nil {
		t.Fatal("Duplicate succeeded with a canceled context")
	}

	// The failed Duplicate must release its reference to the connection.
	if err := runTrue(ctx, hst); err != nil {
		t.Error("Connection is unusable after the failed Duplicate: ", err)
	}
	if err := cc.Close(ctx); err != nil {
		t.Error("cc.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err == nil {
		t.Error("Connection is still open after closing cc")
	}
}

func TestConnCacheDuplicateAfterClose(t *testing.T) {
	ctx := context.Background()
	cc := newTestConnCache(ctx, t)
	if err := cc.Close(ctx); err != nil {
		t.Fatal("cc.Close failed: ", err)
	}
	if _, err := cc.Duplicate(ctx); err == nil {
		t.Error("Duplicate succeeded after Close")
	}
}

func TestConnCacheCloseUnshared(t *testing.T) {
	ctx := context.Background()
	cc := newTestConnCache(ctx, t)
	hst := cc.Conn().SSHConn()
	if err := cc.Close(ctx); err != nil {
		t.Fatal("cc.Close failed: ", err)
	}
	if err := runTrue(ctx, hst); err == nil {
		t.Error("Connection is still open after closing cc")
	}
}
