// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
)

// SSHClient is a Tast gRPC client over an SSH connection.
type SSHClient struct {
	cl  *GenericClient
	cmd *ssh.Cmd
}

// Conn returns a gRPC connection.
func (c *SSHClient) Conn() *grpc.ClientConn {
	return c.cl.Conn()
}

// Close closes this client.
func (c *SSHClient) Close(ctx context.Context) error {
	closeErr := c.cl.Close()
	c.cmd.Abort()
	// Ignore errors from Wait since Abort above causes it to return context.Canceled.
	c.cmd.Wait(ctx)
	return closeErr
}

// DialSSH establishes a gRPC connection to an executable on a remote machine.
func DialSSH(ctx context.Context, conn *ssh.Conn, path string, req *protocol.HandshakeRequest) (*SSHClient, error) {
	cmd := conn.Command(path, "-rpc")
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

	c, err := NewClient(ctx, stdout, stdin, req)
	if err != nil {
		cmd.Abort()
		cmd.Wait(ctx)
		return nil, err
	}
	return &SSHClient{
		cl:  c,
		cmd: cmd,
	}, nil
}

// ExecClient is a Tast gRPC client over a locally executed subprocess.
type ExecClient struct {
	cl  *GenericClient
	cmd *exec.Cmd
}

// Conn returns a gRPC connection.
func (c *ExecClient) Conn() *grpc.ClientConn {
	return c.cl.Conn()
}

// Close closes this client.
func (c *ExecClient) Close() error {
	var firstErr error
	if err := c.cl.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.cmd.Process.Kill(); err != nil && firstErr == nil {
		firstErr = err
	}
	c.cmd.Wait() // ignore error `signal: killed`
	return firstErr
}

// DialExec establishes a gRPC connection to an executable on host.
func DialExec(ctx context.Context, path string, req *protocol.HandshakeRequest) (*ExecClient, error) {
	cmd := exec.CommandContext(ctx, path, "-rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	cmd.Stderr = os.Stderr // ease debug
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("newRemoteFixtureService: %v", err)
	}
	c, err := NewClient(ctx, stdout, stdin, req)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	return &ExecClient{
		cl:  c,
		cmd: cmd,
	}, nil
}

// GenericClient is a Tast gRPC client.
type GenericClient struct {
	conn *grpc.ClientConn
	log  *remoteLoggingClient
}

// Conn returns a gRPC connection.
func (c *GenericClient) Conn() *grpc.ClientConn {
	return c.conn
}

// Close closes this client.
func (c *GenericClient) Close() error {
	var firstErr error
	if err := c.log.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// NewClient establishes a gRPC connection to a test bundle executable using r
// and w.
// Callers are responsible for closing the underlying connection of r/w after
// the client is closed.
func NewClient(ctx context.Context, r io.Reader, w io.Writer, req *protocol.HandshakeRequest) (_ *GenericClient, retErr error) {
	if err := sendRawMessage(w, req); err != nil {
		return nil, err
	}
	res := &protocol.HandshakeResponse{}
	if err := receiveRawMessage(r, res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, errors.Errorf("bundle returned error: %s", res.Error.GetReason())
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

	return &GenericClient{
		conn: conn,
		log:  log,
	}, nil
}

var alwaysAllowedServices = []string{
	"tast.cros.baserpc.FaillogService",
}

// clientOpts returns gRPC client-side interceptors to manipulate context.
func clientOpts() []grpc.DialOption {
	// hook is called on every gRPC method call.
	// It returns a Context to be passed to a gRPC invocation, a function to be
	// called on the end of the gRPC method call to process trailers, and
	// possibly an error.
	hook := func(ctx context.Context, cc *grpc.ClientConn, method string) (context.Context, func(metadata.MD) error, error) {
		if !isUserMethod(method) {
			return ctx, func(metadata.MD) error { return nil }, nil
		}

		// Reject an outgoing RPC call if its service is not declared in ServiceDeps.
		svcs, ok := testcontext.ServiceDeps(ctx)
		if !ok {
			return nil, nil, status.Errorf(codes.FailedPrecondition, "refusing to call %s because ServiceDeps is unavailable (using a wrong context?)", method)
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
			return nil, nil, status.Errorf(codes.FailedPrecondition, "refusing to call %s because it is not declared in ServiceDeps", method)
		}

		after := func(trailer metadata.MD) error {
			var firstErr error
			if err := processTimingTrailer(ctx, trailer.Get(metadataTiming)); err != nil && firstErr == nil {
				firstErr = err
			}
			if err := processOutDirTrailer(ctx, cc, trailer.Get(metadataOutDir)); err != nil && firstErr == nil {
				firstErr = err
			}
			return nil
		}
		return metadata.NewOutgoingContext(ctx, outgoingMetadata(ctx)), after, nil
	}

	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{},
			cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, after, err := hook(ctx, cc, method)
			if err != nil {
				return err
			}

			var trailer metadata.MD
			opts = append([]grpc.CallOption{grpc.Trailer(&trailer)}, opts...)
			retErr := invoker(ctx, method, req, reply, cc, opts...)
			if err := after(trailer); err != nil && retErr == nil {
				retErr = err
			}
			return retErr
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
			method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			ctx, after, err := hook(ctx, cc, method)
			if err != nil {
				return nil, err
			}
			stream, err := streamer(ctx, desc, cc, method, opts...)
			return &clientStreamWithAfter{ClientStream: stream, after: after}, err
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
	after func(trailer metadata.MD) error
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

	if err := s.after(s.Trailer()); err != nil && retErr == io.EOF {
		retErr = err
	}
	return retErr
}
