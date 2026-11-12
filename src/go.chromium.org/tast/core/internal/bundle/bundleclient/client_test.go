// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundleclient

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/rpc"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh"
)

// missingBundle is a bundle name the fake SSH server fails to run.
const missingBundle = "missing"

const pingTimeout = time.Minute

// connectToFakeBundleServer starts an SSH server that serves gRPC like a
// bundle for every command except ones for missingBundle, and returns a
// connection to it.
func connectToFakeBundleServer(ctx context.Context, t *testing.T) *ssh.Conn {
	t.Helper()
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		if strings.Contains(req.Cmd, missingBundle) {
			req.Start(false)
			return
		}
		req.Start(true)
		if err := rpc.RunServer(req, req, nil, func(*grpc.Server, *protocol.HandshakeRequest) error {
			return nil
		}); err != nil {
			fmt.Fprintf(req.Stderr(), "FATAL: %v\n", err)
			req.End(1)
			return
		}
		req.End(0)
	})
	t.Cleanup(td.Close)

	hst, err := ssh.New(ctx, &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatal("ssh.New failed: ", err)
	}
	t.Cleanup(func() { hst.Close(ctx) })
	return hst
}

func TestNewWithSSHConn(t *testing.T) {
	ctx := context.Background()
	hst := connectToFakeBundleServer(ctx, t)
	target := &protocol.TargetDevice{BundleDir: "/bundles"}

	c, err := NewWithSSHConn(ctx, hst, target, "cros", &protocol.HandshakeRequest{})
	if err != nil {
		t.Fatal("NewWithSSHConn failed: ", err)
	}
	if c.SSHConn() != hst {
		t.Error("SSHConn() does not return the given connection")
	}
	if err := c.Close(ctx); err != nil {
		t.Error("Close failed: ", err)
	}
	if c.SSHConn() != nil {
		t.Error("SSHConn() is not nil after Close")
	}
	if err := hst.Ping(ctx, pingTimeout); err != nil {
		t.Error("Given connection is unusable after Close: ", err)
	}
}

func TestNewWithSSHConnFailure(t *testing.T) {
	ctx := context.Background()
	hst := connectToFakeBundleServer(ctx, t)
	target := &protocol.TargetDevice{BundleDir: "/bundles"}

	if _, err := NewWithSSHConn(ctx, hst, target, missingBundle, &protocol.HandshakeRequest{}); err == nil {
		t.Fatal("NewWithSSHConn succeeded for a bundle that fails to run")
	}
	if err := hst.Ping(ctx, pingTimeout); err != nil {
		t.Error("Given connection is unusable after the failure: ", err)
	}
}

func TestCloseClosesOwnedSSHConn(t *testing.T) {
	ctx := context.Background()
	hst := connectToFakeBundleServer(ctx, t)

	c := &Client{hst: hst}
	if err := c.Close(ctx); err != nil {
		t.Fatal("Close failed: ", err)
	}
	if c.SSHConn() != nil {
		t.Error("SSHConn() is not nil after Close")
	}
	if err := hst.Ping(ctx, pingTimeout); err == nil {
		t.Error("Owned connection is still open after Close")
	}
}
