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
	"io/ioutil"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

// RunServer runs a gRPC server on r/w channels.
// register is called back to register core services. svcs is a list of
// user-defined gRPC services to be registered if the client requests them in
// HandshakeRequest.
// RunServer blocks until the client connection is closed or it encounters an
// error.
func RunServer(r io.Reader, w io.Writer, svcs []*testing.Service, register func(srv *grpc.Server)) error {
	var req protocol.HandshakeRequest
	if err := receiveRawMessage(r, &req); err != nil {
		return err
	}

	// Start a remote logging server. It is used to forward logs from
	// user-defined gRPC services via side channels.
	ls := newRemoteLoggingServer()
	srv := grpc.NewServer(serverOpts(ls.Log)...)

	// Register core services.
	reflection.Register(srv)
	protocol.RegisterLoggingServer(srv, ls)
	protocol.RegisterFileTransferServer(srv, newFileTransferServer())
	register(srv)

	// Create a server-scoped context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register user-defined gRPC services if requested.
	if req.GetNeedUserServices() {
		ctx := testcontext.WithLogger(ctx, ls.Log)
		vars := req.GetUserServiceInitParams().GetVars()
		for _, svc := range svcs {
			svc.Register(srv, testing.NewServiceState(ctx, testing.NewServiceRoot(svc, vars)))
		}
	}

	if err := sendRawMessage(w, &protocol.HandshakeResponse{}); err != nil {
		return err
	}

	if err := srv.Serve(NewPipeListener(r, w)); err != nil && err != io.EOF {
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
func serverOpts(logger testcontext.LoggerFunc) []grpc.ServerOption {
	// hook is called on every gRPC method call.
	// It returns a Context to be passed to a gRPC method, a function to be
	// called on the end of the gRPC method call to compute trailers, and
	// possibly an error.
	hook := func(ctx context.Context, method string) (context.Context, func() metadata.MD, error) {
		if !isUserMethod(method) {
			return ctx, func() metadata.MD { return nil }, nil
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, nil, errors.New("metadata not available")
		}

		outDir, err := ioutil.TempDir("", "rpc-outdir.")
		if err != nil {
			return nil, nil, err
		}

		ctx = testcontext.WithLogger(ctx, logger)
		ctx = testcontext.WithCurrentEntity(ctx, incomingCurrentContext(md, outDir))
		tl := timing.NewLog()
		ctx = timing.NewContext(ctx, tl)

		trailer := func() metadata.MD {
			md := make(metadata.MD)

			b, err := json.Marshal(tl)
			if err != nil {
				logger(fmt.Sprint("Failed to marshal timing JSON: ", err))
			} else {
				md[metadataTiming] = []string{string(b)}
			}

			// Send metadataOutDir only if some files were saved in order to avoid extra round-trips.
			if fis, err := ioutil.ReadDir(outDir); err != nil {
				logger(fmt.Sprint("gRPC output directory is corrupted: ", err))
			} else if len(fis) == 0 {
				if err := os.RemoveAll(outDir); err != nil {
					logger(fmt.Sprint("Failed to remove gRPC output directory: ", err))
				}
			} else {
				md[metadataOutDir] = []string{outDir}
			}
			return md
		}
		return ctx, trailer, nil
	}

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
			ctx, trailer, err := hook(ctx, info.FullMethod)
			if err != nil {
				return nil, err
			}
			defer func() {
				grpc.SetTrailer(ctx, trailer())
			}()
			return handler(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx, trailer, err := hook(stream.Context(), info.FullMethod)
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
