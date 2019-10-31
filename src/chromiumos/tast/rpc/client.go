// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// Client owns a gRPC connection to the DUT for remote tests to use.
type Client struct {
	// Conn is the gRPC connection. Use this to create gRPC service stubs.
	Conn *grpc.ClientConn

	log *remoteLoggingClient
	// clean is a function to be called on closing the client.
	// In the typical case of a gRPC connection established over an SSH connection,
	// this function should terminate the test bundle executable running on the DUT.
	clean func(context.Context) error
}

// Close closes this client.
func (c *Client) Close(ctx context.Context) error {
	var firstErr error
	if err := c.log.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
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

	// To avoid data races, log must not be altered while there are active gRPC calls.
	var log *remoteLoggingClient
	waitSeq := func(ctx context.Context, seq int64) error {
		if log == nil {
			return nil
		}
		return log.WaitSeq(ctx, seq)
	}

	conn, err := newPipeClientConn(ctx, r, w, clientOpts(waitSeq)...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish RPC connection")
	}
	defer func() {
		if retErr != nil {
			conn.Close()
		}
	}()

	log, err = newRemoteLoggingClient(ctx, conn, func(msg string) { testing.ContextLog(ctx, msg) })
	if err != nil {
		return nil, errors.Wrap(err, "failed to start remote logging")
	}

	return &Client{
		Conn:  conn,
		log:   log,
		clean: clean,
	}, nil
}

var alwaysAllowedServices = []string{
	"tast.core.Logging",
}

// clientOpts returns gRPC client-side interceptors to manipulate context.
// waitSeq is a function to wait for a log message with a sequence number no less than seq.
func clientOpts(waitSeq func(ctx context.Context, seq int64) error) []grpc.DialOption {
	before := func(ctx context.Context, method string) (context.Context, error) {
		// Reject an outgoing RPC call if its service is not declared in ServiceDeps.
		svcs, ok := testing.ContextServiceDeps(ctx)
		if !ok {
			return nil, status.Errorf(codes.FailedPrecondition, "refusing to call %s because ServiceDeps is unavailable (using a wrong context?)", method)
		}
		svcs = append(svcs, alwaysAllowedServices...)
		matched := false
		for _, svc := range svcs {
			if strings.HasPrefix(method, fmt.Sprintf("/%s/", svc)) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, status.Errorf(codes.FailedPrecondition, "refusing to call %s because it is not declared in ServiceDeps", method)
		}

		md, ok := testing.ContextRPCMetadata(ctx)
		if !ok {
			return nil, status.Errorf(codes.FailedPrecondition, "refusing to call %s because it is called outside from tests", method)
		}
		return metadata.NewOutgoingContext(ctx, md), nil
	}

	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req interface{}, reply interface{},
			cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, err := before(ctx, method)
			if err != nil {
				return err
			}

			var trailer metadata.MD
			opts = append([]grpc.CallOption{grpc.Trailer(&trailer)}, opts...)
			retErr := invoker(ctx, method, req, reply, cc, opts...)
			if seq, ok := lastSeqFromTrailer(trailer); ok {
				if err := waitSeq(ctx, seq); err != nil && retErr == nil {
					retErr = err
				}
			}
			return retErr
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
			method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			ctx, err := before(ctx, method)
			if err != nil {
				return nil, err
			}
			stream, err := streamer(ctx, desc, cc, method, opts...)
			return &waitSeqClientStream{stream, waitSeq}, err
		}),
	}
}

type waitSeqClientStream struct {
	grpc.ClientStream
	waitSeq func(ctx context.Context, seq int64) error
}

func (s *waitSeqClientStream) RecvMsg(m interface{}) error {
	retErr := s.ClientStream.RecvMsg(m)
	if retErr != nil {
		if seq, ok := lastSeqFromTrailer(s.Trailer()); ok {
			if err := s.waitSeq(s.Context(), seq); err != nil {
				retErr = err
			}
		}
	}
	return retErr
}

func lastSeqFromTrailer(md metadata.MD) (int64, bool) {
	if len(md[metadataLastSeq]) != 1 {
		return 0, false
	}
	seq, err := strconv.ParseInt(md[metadataLastSeq][0], 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}
