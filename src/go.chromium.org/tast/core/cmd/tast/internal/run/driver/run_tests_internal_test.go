// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

// Tests for unexported helpers in run_tests.go; run_tests_test.go is in
// package driver_test and cannot access unexported symbols.

import (
	"net"
	"testing"
	"time"

	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh"
)

func TestShouldUseSSHTunnel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sshCfg    *protocol.SSHConfig
		cloudbots string
		want      bool
	}{
		{name: "nil_cfg", sshCfg: nil, want: false},
		{name: "direct", sshCfg: &protocol.SSHConfig{}, want: false},
		{name: "proxy_none", sshCfg: &protocol.SSHConfig{ProxyCommand: "none"}, want: false},
		{name: "proxy_NONE", sshCfg: &protocol.SSHConfig{ProxyCommand: "NONE"}, want: false},
		{name: "proxy_command", sshCfg: &protocol.SSHConfig{ProxyCommand: "ssh -W %h:%p jump"}, want: true},
		{name: "cloudbots", sshCfg: &protocol.SSHConfig{}, cloudbots: "example.com", want: true},
		{name: "cloudbots_with_proxy_none", sshCfg: &protocol.SSHConfig{ProxyCommand: "none"}, cloudbots: "example.com", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLOUDBOTS_LAB_DOMAIN", tc.cloudbots)
			if got := shouldUseSSHTunnel(tc.sshCfg); got != tc.want {
				t.Errorf("shouldUseSSHTunnel(%+v) = %v; want %v", tc.sshCfg, got, tc.want)
			}
		})
	}
}

func TestNewSSHTunnelUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name string
		conn *ssh.Conn
	}{
		{name: "nil_conn", conn: nil},
		{name: "non_ssh_conn", conn: &ssh.Conn{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tunnel := newSSHTunnel(t.Context(), tc.conn); tunnel != nil {
				tunnel.Close()
				t.Fatal("newSSHTunnel returned non-nil tunnel")
			}
		})
	}
}

func TestNewSSHTunnel(t *testing.T) {
	ctx := t.Context()
	userKey, hostKey := sshtest.MustGenerateKeys()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	conn, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{})
	if err != nil {
		t.Fatal("Failed connecting to server: ", err)
	}
	defer conn.Close(ctx)

	tunnel := newSSHTunnel(ctx, conn)
	if tunnel == nil {
		t.Fatal("newSSHTunnel returned nil for active SSH connection")
	}
	addr := tunnel.ListenAddr().String()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		tunnel.Close()
		t.Fatalf("Failed to dial tunnel %s: %v", addr, err)
	}
	c.Close()

	if err := tunnel.Close(); err != nil {
		t.Fatalf("tunnel.Close() failed: %v", err)
	}
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Errorf("Tunnel %s still accepted connections after Close", addr)
	}
}
