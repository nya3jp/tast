// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bundleclient provides a client of test bundles.
package bundleclient

import (
	"context"
	"io"
	"os"

	"google.golang.org/grpc"

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
func (c *Client) dial(ctx context.Context, req *protocol.HandshakeRequest) (_ *rpcConn, retErr error) {
	proc, err := c.cmd.Interact(ctx, []string{"-rpc"})
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
