// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package rpc provides gRPC utilities for Tast tests.
package rpc

import (
	"context"
	"os"
	"path/filepath"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
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
	// Reject dial to a local test bundle with a different name.
	// TODO(b/185755639): Consider dropping bundleName argument entirely.
	exe, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get self bundle name")
	}
	if selfName := filepath.Base(exe); bundleName != selfName {
		return nil, errors.Errorf("cross-bundle gRPC connections are not supported: remote=%q, local=%q", selfName, bundleName)
	}

	bundlePath := filepath.Join(testing.ExtractLocalBundleDir(h), bundleName)
	req := &protocol.HandshakeRequest{
		NeedUserServices: true,
		UserServiceInitParams: &protocol.UserServiceInitParams{
			Vars: testing.ExtractTestVars(h),
		},
	}
	return rpc.DialSSH(ctx, d.Conn(), bundlePath, req)
}
