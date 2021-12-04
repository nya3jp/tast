// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

// RunServer runs a gRPC server on r/w channels.
// register is called back to register core services. svcs is a list of
// user-defined gRPC services to be registered if the client requests them in
// HandshakeRequest.
// RunServer blocks until the client connection is closed or it encounters an
// error.
func RunServer(r io.Reader, w io.Writer, svcs []*testing.Service, register func(srv *grpc.Server, req *protocol.HandshakeRequest) error) error {
	// In case w is stdout or stderr, writing data to it after it is closed
	// causes SIGPIPE to be delivered to the process, which by default
	// terminates the process without running deferred cleanup calls.
	// To avoid the issue, ignore SIGPIPE while running the gRPC server.
	// See https://golang.org/pkg/os/signal/#hdr-SIGPIPE for more details.
	signal.Ignore(unix.SIGPIPE)
	defer signal.Reset(unix.SIGPIPE)

	var req protocol.HandshakeRequest
	if err := receiveRawMessage(r, &req); err != nil {
		return err
	}

	// Make sure to return only after all active method calls finish.
	// Otherwise the process can exit before running deferred function
	// calls on service goroutines.
	var calls sync.WaitGroup
	defer calls.Wait()

	// Start a remote logging server. It is used to forward logs from
	// user-defined gRPC services via side channels.
	ls := newRemoteLoggingServer()
	srv := grpc.NewServer(serverOpts(ls, &calls)...)

	// Register core services.
	regErr := registerCoreServices(srv, ls, &req, register)

	// Create a server-scoped context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register user-defined gRPC services if requested.
	if req.GetNeedUserServices() {
		registerUserServices(ctx, srv, ls, &req, svcs, false)
	}

	if regErr != nil {
		err := errors.Wrap(regErr, "gRPC server initialization failed")
		res := &protocol.HandshakeResponse{
			Error: &protocol.HandshakeError{
				Reason: fmt.Sprintf("gRPC server initialization failed: %v", err),
			},
		}
		sendRawMessage(w, res)
		return err
	}

	if err := sendRawMessage(w, &protocol.HandshakeResponse{}); err != nil {
		return err
	}

	// From now on, catch SIGINT/SIGTERM to stop the server gracefully.
	sigCh := make(chan os.Signal, 1)
	defer close(sigCh)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	defer signal.Stop(sigCh)
	sigErrCh := make(chan error, 1)
	go func() {
		if sig, ok := <-sigCh; ok {
			sigErrCh <- errors.Errorf("caught signal %d (%s)", sig, sig)
			srv.Stop()
		}
	}()

	if err := srv.Serve(NewPipeListener(r, w)); err != nil && err != io.EOF {
		// Replace the error if we saw a signal.
		select {
		case err := <-sigErrCh:
			return err
		default:
		}
		return err
	}
	return nil
}

// RunTCPServer runs a gRPC server listening on the specified port thought TCP
// Port contains the TCP port number where gRPC server listens to
// HandshakeRequest contains parameters needed to initialize a gRPC server.
// svcs is the candidate list of user-defined gRPC services and they will be
// registered if GuaranteeCompatibility is set.
func RunTCPServer(port int, handshakeReq *protocol.HandshakeRequest, svcs []*testing.Service,
	register func(srv *grpc.Server, req *protocol.HandshakeRequest) error) error {
	// Make sure to return only after all active method calls finish.
	// Otherwise the process can exit before running deferred function
	// calls on service goroutines.
	var calls sync.WaitGroup
	defer calls.Wait()

	// Start a remote logging server. It is used to forward logs from
	// user-defined gRPC services via side channels.
	ls := newRemoteLoggingServer()
	srv := grpc.NewServer(serverOpts(ls, &calls)...)

	// Register core services.
	regErr := registerCoreServices(srv, ls, handshakeReq, register)

	if regErr != nil {
		return errors.Wrap(regErr, "gRPC server initialization failed")
	}

	// Create a server-scoped context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register user-defined gRPC services intended for public use.
	registerUserServices(ctx, srv, ls, handshakeReq, svcs, true)

	// From now on, catch SIGINT/SIGTERM to stop the server gracefully.
	sigCh := make(chan os.Signal, 1)
	defer close(sigCh)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	defer signal.Stop(sigCh)
	sigErrCh := make(chan error, 1)
	go func() {
		if sig, ok := <-sigCh; ok {
			sigErrCh <- errors.Errorf("caught signal %d (%s)", sig, sig)
			srv.Stop()
		}
	}()

	// start gRPC server listening on the tcp port
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return errors.Wrap(err, "server failed to listen")
	}

	if err := srv.Serve(listener); err != nil && err != io.EOF {
		// Replace the error if we saw a signal.
		select {
		case err := <-sigErrCh:
			return err
		default:
		}
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
func serverOpts(ls *remoteLoggingServer, calls *sync.WaitGroup) []grpc.ServerOption {
	// hook is called on every gRPC method call.
	// It returns a Context to be passed to a gRPC method, a function to be
	// called on the end of the gRPC method call to compute trailers, and
	// possibly an error.

	hook := func(ctx context.Context, method string) (context.Context, func() metadata.MD, error) {
		// Forward all uncaptured logs via LoggingService.
		ctx = logging.AttachLogger(ctx, logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(ls.Log)))

		var outDir string
		var tl *timing.Log
		if isUserMethod(method) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				return nil, nil, errors.New("metadata not available")
			}

			var err error
			outDir, err = ioutil.TempDir("", "rpc-outdir.")
			if err != nil {
				return nil, nil, err
			}

			// Make the directory world-writable so that tests can create files as other users,
			// and set the sticky bit to prevent users from deleting other users' files.
			if err := os.Chmod(outDir, 0777|os.ModeSticky); err != nil {
				return nil, nil, err
			}

			ctx = testcontext.WithCurrentEntity(ctx, incomingCurrentContext(md, outDir))
			tl = timing.NewLog()
			ctx = timing.NewContext(ctx, tl)
		}

		trailer := func() metadata.MD {
			md := make(metadata.MD)

			if isUserMethod(method) {
				b, err := json.Marshal(tl)
				if err != nil {
					logging.Info(ctx, "Failed to marshal timing JSON: ", err)
				} else {
					md[metadataTiming] = []string{string(b)}
				}

				// Send metadataOutDir only if some files were saved in order to avoid extra round-trips.
				if fis, err := ioutil.ReadDir(outDir); err != nil {
					logging.Info(ctx, "gRPC output directory is corrupted: ", err)
				} else if len(fis) == 0 {
					if err := os.RemoveAll(outDir); err != nil {
						logging.Info(ctx, "Failed to remove gRPC output directory: ", err)
					}
				} else {
					md[metadataOutDir] = []string{outDir}
				}
			}

			if !isLoggingMethod(method) {
				md[metadataLogLastSeq] = []string{strconv.FormatUint(ls.LastSeq(), 10)}
			}
			return md
		}
		return ctx, trailer, nil
	}

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
			calls.Add(1)
			defer calls.Done()
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
			calls.Add(1)
			defer calls.Done()
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

// registerCoreServices registers core Tast services.
// srv is the gRPC server instance
// ls is the remote logging server that forwards logs through side channel
// HandshakeRequest contains parameters needed to initialize a gRPC server.
// svcs is the candidate list of user-defined gRPC services to be registered
// register offers a callback hook for additional service registration
func registerCoreServices(srv *grpc.Server, ls *remoteLoggingServer,
	handshakeReq *protocol.HandshakeRequest, register func(srv *grpc.Server, req *protocol.HandshakeRequest) error) error {
	reflection.Register(srv)
	protocol.RegisterLoggingServer(srv, ls)
	protocol.RegisterFileTransferServer(srv, newFileTransferServer())
	return register(srv, handshakeReq)
}

// registerUserServices registers user defined gRPC services to the gRPC Server
// srv is the gRPC server instance
// ls is the remote logging server that forwards logs through side channel
// HandshakeRequest contains parameters needed to initialize a gRPC server.
// svcs is the candidate list of user-defined gRPC services to be registered
// guaranteeCompatibilityOnly determines if the service registration is restricted
// only to services with GuaranteeCompatibility set
func registerUserServices(ctx context.Context, srv *grpc.Server, ls *remoteLoggingServer,
	handshakeReq *protocol.HandshakeRequest, svcs []*testing.Service, guaranteeCompatibilityOnly bool) error {
	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(ls.Log))
	ctx = logging.AttachLogger(ctx, logger)
	vars := handshakeReq.GetBundleInitParams().GetVars()
	for _, svc := range svcs {
		if !guaranteeCompatibilityOnly || svc.GuaranteeCompatibility {
			svc.Register(srv, testing.NewServiceState(ctx, testing.NewServiceRoot(svc, vars)))
		}
	}
	return nil
}

// startServing kicks off the gRPC server listening through the listener
func startServing(srv *grpc.Server, listener net.Listener) error {
	// From now on, catch SIGINT/SIGTERM to stop the server gracefully.
	sigCh := make(chan os.Signal, 1)
	defer close(sigCh)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	defer signal.Stop(sigCh)
	sigErrCh := make(chan error, 1)
	go func() {
		if sig, ok := <-sigCh; ok {
			sigErrCh <- errors.Errorf("caught signal %d (%s)", sig, sig)
			srv.Stop()
		}
	}()

	if err := srv.Serve(listener); err != nil {
		// Replace the error if we saw a signal.
		select {
		case err := <-sigErrCh:
			return err
		default:
		}
		return err
	}
	return nil
}
