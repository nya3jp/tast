// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"
	"path/filepath"

	"google.golang.org/grpc"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// Client owns a gRPC connection to the DUT for remote tests to use.
type Client struct {
	// Conn is the gRPC connection. Use this to create gRPC service stubs.
	Conn *grpc.ClientConn

	// clean is a function to be called on closing the client.
	// In the typical case of a gRPC connection established over an SSH connection,
	// this function should terminate the test bundle executable running on the DUT.
	clean func(context.Context) error
}

// Close closes this client.
func (c *Client) Close(ctx context.Context) error {
	var firstErr error
	if err := c.Conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.clean(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

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
	bundlePath := filepath.Join(h.LocalBundleDir, bundleName)
	cmd := d.Command(bundlePath, "-rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to connect to RPC service on DUT")
	}

	return newClient(ctx, stdout, stdin, func(ctx context.Context) error {
		cmd.Abort()
		return cmd.Wait(ctx)
	})
}

// newClient establishes a gRPC connection to a test bundle executable using r and w.
//
// When this function succeeds, clean is called in Client.Close. Otherwise it is called
// before this function returns.
func newClient(ctx context.Context, r io.Reader, w io.Writer, clean func(context.Context) error) (_ *Client, retErr error) {
	defer func() {
		if retErr != nil {
			clean(ctx)
		}
	}()

	conn, err := newPipeClientConn(ctx, r, w, clientOpts()...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish RPC connection")
	}
	defer func() {
		if retErr != nil {
			conn.Close()
		}
	}()

	return &Client{
		Conn:  conn,
		clean: clean,
	}, nil
}

// clientOpts returns gRPC client-side interceptors to manipulate context.
func clientOpts() []grpc.DialOption {
	before := func(ctx context.Context, method string) (context.Context, error) {
		// TODO(crbug.com/969627): Replace ctx to forward testing.TestContext.
		return ctx, nil
	}

	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req interface{}, reply interface{},
			cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, err := before(ctx, method)
			if err != nil {
				return err
			}
			return invoker(ctx, method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
			method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			ctx, err := before(ctx, method)
			if err != nil {
				return nil, err
			}
			return streamer(ctx, desc, cc, method, opts...)
		}),
	}
}
