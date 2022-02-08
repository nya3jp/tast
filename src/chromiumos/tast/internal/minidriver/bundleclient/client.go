// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bundleclient provides a client of test bundles.
package bundleclient

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/minidriver/target"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/run/genericexec"
)

// rpcConn represents a gRPC connection to a test bundle.
type rpcConn struct {
	proc genericexec.Process
	conn *rpc.GenericClient
}

// Close closes the gRPC connection to the test bundle.
func (c *rpcConn) Close(ctx context.Context) error {
	var firstErr error
	if err := c.conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.proc.Stdin().Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.proc.Wait(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Conn returns the established gRPC connection.
func (c *rpcConn) Conn() *grpc.ClientConn {
	return c.conn.Conn()
}

// Client is a gRPC protocol client to a test bundle.
type Client struct {
	cmd genericexec.Cmd
}

// New creates a new Client.
func New(cmd genericexec.Cmd) *Client {
	return &Client{
		cmd: cmd,
	}
}

// dial connects to the test bundle and established a gRPC connection.
func (c *Client) dial(ctx context.Context, req *protocol.HandshakeRequest, debugPort int) (_ *rpcConn, retErr error) {
	debugCmd, err := c.cmd.DebugCommand(ctx, debugPort)
	if err != nil {
		return nil, err
	}
	proc, err := debugCmd.Interact(ctx, []string{"-rpc"})
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			proc.Stdin().Close()
			proc.Wait(ctx)
		}
	}()

	// Pass through stderr.
	go io.Copy(os.Stderr, proc.Stderr())

	conn, err := rpc.NewClient(ctx, proc.Stdout(), proc.Stdin(), req)
	if err != nil {
		return nil, err
	}

	return &rpcConn{
		proc: proc,
		conn: conn,
	}, nil
}

// LocalCommand creates a SSH command to run exec on the target specified by cc.
func LocalCommand(exec string, proxy bool, cc *target.ConnCache) *genericexec.SSHCmd {
	var args []string
	// The delve debugger attempts to write to a directory not on the stateful partition.
	// This ensures it instead writes to the stateful partition.
	if proxy {
		// Proxy-related variables can be either uppercase or lowercase.
		// See https://golang.org/pkg/net/http/#ProxyFromEnvironment.
		for _, name := range []string{
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
			"http_proxy", "https_proxy", "no_proxy",
		} {
			if val := os.Getenv(name); val != "" {
				args = append(args, fmt.Sprintf("%s=%s", name, val))
			}
		}
	}
	args = append(args, exec)

	cmd := genericexec.CommandSSH(cc.Conn().SSHConn(), "env", args...)
	return cmd
}

// NewLocal creates a bundle client to the local bundle.
func NewLocal(bundle, bundleDir string, proxy bool, cc *target.ConnCache) *Client {
	cmd := LocalCommand(filepath.Join(bundleDir, bundle), proxy, cc)
	return New(cmd)
}
