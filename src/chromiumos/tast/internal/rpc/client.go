// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
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
	bundlePath := filepath.Join(testing.ExtractLocalBundleDir(h), bundleName)
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

	return newClient(ctx, stdout, stdin, h, func(ctx context.Context) error {
		cmd.Abort()
		return cmd.Wait(ctx)
	})
}

// newClient establishes a gRPC connection to a test bundle executable using r and w.
//
// When this function succeeds, clean is called in Client.Close. Otherwise it is called
// before this function returns.
func newClient(ctx context.Context, r io.Reader, w io.Writer, h *testing.RPCHint, clean func(context.Context) error) (_ *Client, retErr error) {
	defer func() {
		if retErr != nil {
			clean(ctx)
		}
	}()

	if err := initBundleServer(r, w, h); err != nil {
		return nil, errors.Wrap(err, "failed to set bundle parameters")
	}

	conn, err := NewPipeClientConn(ctx, r, w, clientOpts()...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish RPC connection")
	}
	defer func() {
		if retErr != nil {
			conn.Close()
		}
	}()

	log, err := newRemoteLoggingClient(ctx, conn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start remote logging")
	}

	return &Client{
		Conn:  conn,
		log:   log,
		clean: clean,
	}, nil
}

// initBundleServer initializes the bundle server by sending raw protobuf message
// to bundle process, and waits for response message.
func initBundleServer(r io.Reader, w io.Writer, h *testing.RPCHint) error {
	req := &protocol.HandshakeRequest{
		UserServiceInitParams: &protocol.UserServiceInitParams{
			Vars: testing.ExtractTestVars(h),
		},
	}
	if err := sendRawMessage(w, req); err != nil {
		return err
	}
	res := &protocol.HandshakeResponse{}
	if err := receiveRawMessage(r, res); err != nil {
		return err
	}
	// Server returns error.
	if res.Error != nil {
		return errors.Errorf("bundle returned error: %s", res.Error.GetReason())
	}
	return nil
}

var alwaysAllowedServices = []string{
	"tast.core.Logging",
	"tast.core.FileTransfer",
	"tast.cros.baserpc.FaillogService",
}

// clientOpts returns gRPC client-side interceptors to manipulate context.
func clientOpts() []grpc.DialOption {
	before := func(ctx context.Context, method string) (context.Context, error) {
		// Reject an outgoing RPC call if its service is not declared in ServiceDeps.
		svcs, ok := testcontext.ServiceDeps(ctx)
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

		md := outgoingMetadata(ctx)
		return metadata.NewOutgoingContext(ctx, md), nil
	}

	after := func(ctx context.Context, cc *grpc.ClientConn, trailer metadata.MD) error {
		var firstErr error
		if err := processTimingTrailer(ctx, trailer.Get(metadataTiming)); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := processOutDirTrailer(ctx, cc, trailer.Get(metadataOutDir)); err != nil && firstErr == nil {
			firstErr = err
		}
		return nil
	}

	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{},
			cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, err := before(ctx, method)
			if err != nil {
				return err
			}

			var trailer metadata.MD
			opts = append([]grpc.CallOption{grpc.Trailer(&trailer)}, opts...)
			retErr := invoker(ctx, method, req, reply, cc, opts...)
			if err := after(ctx, cc, trailer); err != nil && retErr == nil {
				retErr = err
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
			return &clientStreamWithAfter{ClientStream: stream, cc: cc, after: after}, err
		}),
	}
}

func processTimingTrailer(ctx context.Context, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) >= 2 {
		return errors.Errorf("gRPC trailer %s contains %d values", metadataTiming, len(values))
	}

	var tl timing.Log
	if err := json.Unmarshal([]byte(values[0]), &tl); err != nil {
		return errors.Wrapf(err, "failed to parse gRPC trailer %s", metadataTiming)
	}
	if _, stg, ok := timing.FromContext(ctx); ok {
		if err := stg.Import(&tl); err != nil {
			return errors.Wrap(err, "failed to import gRPC timing log")
		}
	}
	return nil
}

func processOutDirTrailer(ctx context.Context, cc *grpc.ClientConn, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) >= 2 {
		return errors.Errorf("gRPC trailer %s contains %d values", metadataOutDir, len(values))
	}

	src := values[0]
	dst, ok := testcontext.OutDir(ctx)
	if !ok {
		return errors.New("output directory not associated to the context")
	}

	if err := pullDirectory(ctx, protocol.NewFileTransferClient(cc), src, dst); err != nil {
		return errors.Wrap(err, "failed to pull output files from gRPC service")
	}
	return nil
}

// clientStreamWithAfter wraps grpc.ClientStream with a function to be called
// on the end of the streaming call.
type clientStreamWithAfter struct {
	grpc.ClientStream
	cc    *grpc.ClientConn
	after func(ctx context.Context, cc *grpc.ClientConn, trailer metadata.MD) error
	done  bool
}

func (s *clientStreamWithAfter) RecvMsg(m interface{}) error {
	retErr := s.ClientStream.RecvMsg(m)
	if retErr == nil {
		return nil
	}

	if s.done {
		return retErr
	}
	s.done = true

	if err := s.after(s.Context(), s.cc, s.Trailer()); err != nil && retErr == io.EOF {
		retErr = err
	}
	return retErr
}
