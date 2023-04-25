// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides gRPC utilities for Tast tests.
package rpc

import (
	"context"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/rpc"
)

// Client owns a gRPC connection to the DUT for remote tests to use.
type Client = rpc.Client

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
	return rpc.Dial(ctx, d, h)
}
