// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
)

// runRPCServer runs a gRPC server on stdin and stdout.
func runRPCServer(stdin io.Reader, stdout io.Writer) error {
	srv := grpc.NewServer(interceptors()...)
	reflection.Register(srv)

	for _, s := range testing.GlobalRegistry().AllServices() {
		s.Register(srv, &testing.ServiceState{})
	}

	if err := srv.Serve(rpc.NewPipeListener(stdin, stdout)); err != nil && err != io.EOF {
		return err
	}
	return nil
}

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context {
	return s.ctx
}

var _ grpc.ServerStream = (*serverStreamWithContext)(nil)

func interceptors() []grpc.ServerOption {
	before := func(ctx context.Context) (context.Context, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errors.New("metadata not available")
		}
		tc, err := testing.TestContextFromRPCMetadata(md)
		if err != nil {
			return nil, err
		}
		return testing.WithTestContext(ctx, tc), nil
	}

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
			ctx, err = before(ctx)
			if err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx, err := before(stream.Context())
			if err != nil {
				return err
			}
			stream = &serverStreamWithContext{stream, ctx}
			return handler(srv, stream)
		}),
	}
}
