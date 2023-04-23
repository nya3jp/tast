// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides gRPC utilities for Tast tests.
package rpc

import (
	"context"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	"chromiumos/tast/ssh"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/tastuseonly/protocol"
	"go.chromium.org/tast/core/tastuseonly/rpc"
	"go.chromium.org/tast/core/tastuseonly/testing"
)

// Client owns a gRPC connection to the DUT for remote tests to use.
type Client struct {
	// Conn is the gRPC connection. Use this to create gRPC service stubs.
	Conn *grpc.ClientConn

	cl *rpc.SSHClient
}

// Close closes the connection.
// TODO(b/3042409): Remove ctx param from this method.
func (c *Client) Close(ctx context.Context, opts ...ssh.RunOption) error {
	return c.cl.Close(opts...)
}

// Dial establishes a gRPC connection to the test bundle executable
// using d and h.
//
// The context passed in must remain valid for as long as the gRPC connection.
// I.e. Don't use the context from within a testing.Poll function.
//
// Example:
//
//	cl, err := rpc.Dial(ctx, d, s.RPCHint())
//	if err != nil {
//		return err
//	}
//	defer cl.Close(ctx)
//
//	fs := base.NewFileSystemClient(cl.Conn)
//
//	res, err := fs.ReadDir(ctx, &base.ReadDirRequest{Dir: "/mnt/stateful_partition"})
//	if err != nil {
//		return err
//	}
func Dial(ctx context.Context, d *dut.DUT, h *testing.RPCHint) (*Client, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get self bundle name")
	}

	selfName := filepath.Base(exe)

	bundlePath := filepath.Join(testing.ExtractLocalBundleDir(h), selfName)
	req := &protocol.HandshakeRequest{
		NeedUserServices: true,
		BundleInitParams: &protocol.BundleInitParams{
			Vars: testing.ExtractTestVars(h),
		},
	}
	cl, err := rpc.DialSSH(ctx, d.Conn(), bundlePath, req, false)
	if err != nil {
		return nil, err
	}
	return &Client{
		Conn: cl.Conn(),
		cl:   cl,
	}, nil
}
