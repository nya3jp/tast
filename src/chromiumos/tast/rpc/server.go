// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

// RunServer runs a gRPC server providing svcs on r/w channels.
// It blocks until the client connection is closed or it encounters an error.
func RunServer(r io.Reader, w io.Writer, svcs []*testing.Service) error {
	ls := newRemoteLoggingServer()
	srv := grpc.NewServer(serverOpts(ls.Log)...)
	RegisterLoggingServer(srv, ls)

	// Register the reflection service for easier debugging.
	reflection.Register(srv)

	for _, svc := range svcs {
		svc.Register(srv, &testing.ServiceState{})
	}

	if err := srv.Serve(newPipeListener(r, w)); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// serverStreamWithContext wraps grpc.ServerStream with overriding Context.
type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

// Context overrides grpc.ServerStream.Context.
func (s *serverStreamWithContext) Context() context.Context {
	return s.ctx
}

var _ grpc.ServerStream = (*serverStreamWithContext)(nil)

// serverOpts returns gRPC server-side interceptors to manipulate context.
func serverOpts(logger func(msg string)) []grpc.ServerOption {
	var tl *timing.Log
	before := func(ctx context.Context) (context.Context, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errors.New("metadata not available")
		}
		tc := incomingTestContext(md, logger)
		ctx = testing.WithTestContext(ctx, tc)
		tl = timing.NewLog()
		ctx = timing.NewContext(ctx, tl)
		return ctx, nil
	}
	trailer := func() metadata.MD {
		b, err := json.Marshal(tl)
		if err != nil {
			logger(fmt.Sprint("Failed to marshal timing JSON: ", err))
			return nil
		}
		return metadata.Pairs(metadataTiming, string(b))
	}

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
			ctx, err = before(ctx)
			if err != nil {
				return nil, err
			}
			defer func() {
				grpc.SetTrailer(ctx, trailer())
			}()
			return handler(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx, err := before(stream.Context())
			if err != nil {
				return err
			}
			stream = &serverStreamWithContext{stream, ctx}
			defer func() {
				stream.SetTrailer(trailer())
			}()
			return handler(srv, stream)
		}),
	}
}
