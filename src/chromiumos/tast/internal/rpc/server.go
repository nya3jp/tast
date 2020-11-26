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

// RunServer runs a gRPC server providing svcs on r/w channels.
// It blocks until the client connection is closed or it encounters an error.
func RunServer(r io.Reader, w io.Writer, svcs []*testing.Service) error {
	// Prepare and get bundle init message, and extract runtime vars.
	initReq, err := prepareServer(r, w)
	if err != nil {
		return err
	}
	vars := initReq.GetVars()

	ls := newRemoteLoggingServer()
	srv := grpc.NewServer(serverOpts(ls.Log)...)
	protocol.RegisterLoggingServer(srv, ls)
	protocol.RegisterFileTransferServer(srv, newFileTransferServer())

	// Register the reflection service for easier debugging.
	reflection.Register(srv)

	// Create a service-scoped context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testcontext.WithLogger(ctx, ls.Log)

	for _, svc := range svcs {
		svc.Register(srv, testing.NewServiceState(ctx, testing.NewServiceRoot(svc, vars)))
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
	before := func(ctx context.Context) (context.Context, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errors.New("metadata not available")
		}
		outDir, err := ioutil.TempDir("", "rpc-outdir.")
		if err != nil {
			return nil, err
		}
		ctx = testcontext.WithLogger(ctx, logger)
		ctx = testcontext.WithCurrentEntity(ctx, incomingCurrentContext(md, outDir))
		ctx = timing.NewContext(ctx, timing.NewLog())
		return ctx, nil
	}
	trailer := func(ctx context.Context) metadata.MD {
		// Note: ctx passed here is one created by "before" above. Thus we can
		// assume that functions to extract values from the context always
		// succeed.
		md := make(metadata.MD)

		tl, _, _ := timing.FromContext(ctx)
		b, err := json.Marshal(tl)
		if err != nil {
			logger(fmt.Sprint("Failed to marshal timing JSON: ", err))
		} else {
			md[metadataTiming] = []string{string(b)}
		}

		outDir, _ := testcontext.OutDir(ctx)
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

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
			ctx, err = before(ctx)
			if err != nil {
				return nil, err
			}
			defer func() {
				grpc.SetTrailer(ctx, trailer(ctx))
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
				stream.SetTrailer(trailer(ctx))
			}()
			return handler(srv, stream)
		}),
	}
}

// prepareServer obtains RPC init data from the RPC client.
func prepareServer(r io.Reader, w io.Writer) (*protocol.InitBundleServerRequest, error) {
	initReq := &protocol.InitBundleServerRequest{}
	err := receiveRawMessage(r, initReq)
	initRsp := &protocol.InitBundleServerResponse{
		Success: err == nil,
	}
	if err != nil {
		initRsp.ErrorMessage = err.Error()
	}
	return initReq, sendRawMessage(w, initRsp)
}
