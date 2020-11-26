// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides gRPC utilities for Tast tests.
package rpc

import (
	"context"

	"chromiumos/tast/dut"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// Client owns a gRPC connection to the DUT for remote tests to use.
type Client = rpc.Client

// Dial establishes a gRPC connection to the test bundle executable named
// bundleName using d and h.
//
// Example:
//
//  cl, err := rpc.Dial(ctx, d, s.RPCHint(), "cros")
//  if err != nil {
//  	return err
//  }
//  defer cl.Close(ctx)
//
//  fs := base.NewFileSystemClient(cl.Conn)
//
//  res, err := fs.ReadDir(ctx, &base.ReadDirRequest{Dir: "/mnt/stateful_partition"})
//  if err != nil {
//  	return err
//  }
func Dial(ctx context.Context, d *dut.DUT, h *testing.RPCHint, bundleName string) (*Client, error) {
	return rpc.Dial(ctx, d, h, bundleName)
}
