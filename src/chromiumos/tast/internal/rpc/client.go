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
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
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
func (c *SSHClient) Close() error {
	closeErr := c.cl.Close()
	c.cmd.Abort()
	// Ignore errors from Wait since Abort above causes it to return context.Canceled.
	c.cmd.Wait()
	return closeErr
}

// DialSSH establishes a gRPC connection to an executable on a remote machine.
// proxy if true indicates that HTTP proxy environment variables should be forwarded.
func DialSSH(ctx context.Context, conn *ssh.Conn, path string, req *protocol.HandshakeRequest, proxy bool) (*SSHClient, error) {
	args := []string{path, "-rpc"}
	if proxy {
		var envArgs []string
		// Proxy-related variables can be either uppercase or lowercase.
		// See https://golang.org/pkg/net/http/#ProxyFromEnvironment.
		for _, name := range []string{
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
			"http_proxy", "https_proxy", "no_proxy",
		} {
			if val := os.Getenv(name); val != "" {
				envArgs = append(envArgs, fmt.Sprintf("%s=%s", name, val))
			}
		}
		args = append(append([]string{"env"}, envArgs...), args...)
	}
	cmd := conn.CommandContext(ctx, args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to connect to RPC service on DUT")
	}

	c, err := NewClient(ctx, stdout, stdin, req)
	if err != nil {
		cmd.Abort()
		cmd.Wait()
		return nil, err
	}
	return &SSHClient{
		cl:  c,
		cmd: cmd,
	}, nil
}

// ExecClient is a Tast gRPC client over a locally executed subprocess.
type ExecClient struct {
	cl         *GenericClient
	cmd        *exec.Cmd
	newSession bool
}

// Conn returns a gRPC connection.
func (c *ExecClient) Conn() *grpc.ClientConn {
	return c.cl.Conn()
}

// PID returns PID of the subprocess.
func (c *ExecClient) PID() int {
	return c.cmd.Process.Pid
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
	if c.newSession {
		killSession(c.cmd.Process.Pid)
	}
	c.cmd.Wait() // ignore error `signal: killed`
	return firstErr
}

// DialExec establishes a gRPC connection to an executable on host.
// If newSession is true, a new session is created for the subprocess and its
// descendants so that all of them are killed on closing Client.
func DialExec(ctx context.Context, path string, newSession bool, req *protocol.HandshakeRequest) (*ExecClient, error) {
	cmd := exec.CommandContext(ctx, path, "-rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run %s for RPC", path)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run %s for RPC", path)
	}
	cmd.Stderr = os.Stderr // ease debug
	if newSession {
		cmd.SysProcAttr = &unix.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "failed to run %s for RPC", path)
	}
	c, err := NewClient(ctx, stdout, stdin, req)
	if err != nil {
		cmd.Process.Kill()
		if newSession {
			killSession(cmd.Process.Pid)
		}
		cmd.Wait()
		return nil, err
	}
	return &ExecClient{
		cl:         c,
		cmd:        cmd,
		newSession: newSession,
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
func NewClient(ctx context.Context, r io.Reader, w io.Writer, req *protocol.HandshakeRequest, opts ...grpc.DialOption) (_ *GenericClient, retErr error) {
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

	lazyLog := newLazyRemoteLoggingClient()
	conn, err := NewPipeClientConn(ctx, r, w, append(clientOpts(lazyLog), opts...)...)
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

	lazyLog.SetClient(log)

	return &GenericClient{
		conn: conn,
		log:  log,
	}, nil
}

var alwaysAllowedServices = []string{
	"tast.cros.baserpc.FaillogService",
	"tast.cros.baserpc.FileSystem",
}

// clientOpts returns gRPC client-side interceptors to manipulate context.
func clientOpts(lazyLog *lazyRemoteLoggingClient) []grpc.DialOption {
	// hook is called on every gRPC method call.
	// It returns a Context to be passed to a gRPC invocation, a function to be
	// called on the end of the gRPC method call to process trailers, and
	// possibly an error.
	hook := func(ctx context.Context, cc *grpc.ClientConn, method string) (context.Context, func(metadata.MD) error, error) {
		if isUserMethod(method) {
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
		}

		after := func(trailer metadata.MD) error {
			var firstErr error
			if isUserMethod(method) {
				if err := processTimingTrailer(ctx, trailer.Get(metadataTiming)); err != nil && firstErr == nil {
					firstErr = err
				}
				if err := processOutDirTrailer(ctx, cc, trailer.Get(metadataOutDir)); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			if !isLoggingMethod(method) {
				if err := processLoggingTrailer(ctx, lazyLog, trailer.Get(metadataLogLastSeq)); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
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

func processLoggingTrailer(ctx context.Context, lazyLog *lazyRemoteLoggingClient, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) >= 2 {
		return errors.Errorf("gRPC trailer %s contains %d values", metadataLogLastSeq, len(values))
	}

	seq, err := strconv.ParseUint(values[0], 10, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse gRPC trailer %s", metadataLogLastSeq)
	}

	if err := lazyLog.Wait(ctx, seq); err != nil {
		return errors.Wrap(err, "failed to wait for pending logs")
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

// lazyRemoteLoggingClient wraps remoteLoggingClient for lazy initialization.
// We have to install logging hooks on starting a gRPC connection, but
// remoteLoggingClient can be started only after a gRPC connection is ready.
// lazyRemoteLoggingClient allows logging hooks to access remoteLoggingClient
// after it becomes available.
// lazyRemoteLoggingClient is goroutine-safe.
type lazyRemoteLoggingClient struct {
	client atomic.Value
}

func newLazyRemoteLoggingClient() *lazyRemoteLoggingClient {
	return &lazyRemoteLoggingClient{}
}

func (l *lazyRemoteLoggingClient) SetClient(client *remoteLoggingClient) {
	l.client.Store(client)
}

func (l *lazyRemoteLoggingClient) Wait(ctx context.Context, seq uint64) error {
	client, ok := l.client.Load().(*remoteLoggingClient)
	if !ok {
		return nil
	}
	return client.Wait(ctx, seq)
}

// killSession makes a best-effort attempt to kill all processes in session sid.
// It makes several passes over the list of running processes, sending sig to any
// that are part of the session. After it doesn't find any new processes, it returns.
// Note that this is racy: it's possible (but hopefully unlikely) that continually-forking
// processes could spawn children that don't get killed.
func killSession(sid int) {
	const maxPasses = 3
	for i := 0; i < maxPasses; i++ {
		pids, err := process.Pids()
		if err != nil {
			return
		}
		n := 0
		for _, pid := range pids {
			pid := int(pid)
			if s, err := unix.Getsid(pid); err == nil && s == sid {
				unix.Kill(pid, unix.SIGKILL)
				n++
			}
		}
		// If we didn't find any processes in the session, we're done.
		if n == 0 {
			return
		}
	}
}
