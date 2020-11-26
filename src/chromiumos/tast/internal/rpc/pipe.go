// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"chromiumos/tast/errors"
)

var (
	// fakeAddr is a fake IPv4 address.
	fakeAddr = &net.IPAddr{IP: net.IPv4zero}

	// errNotImpl is returned from unimplemented methods in pipeConn.
	errNotImpl = errors.New("not implemented")
)

// pipeConn is a pseudo net.Conn implementation based on io.Reader and io.Writer.
type pipeConn struct {
	r io.Reader
	w io.Writer
	c func() error // if not nil, called on the first Close

	closed bool       // true after Close is called
	mu     sync.Mutex // protects closed
}

// Read reads data from the underlying io.Reader.
func (c *pipeConn) Read(b []byte) (n int, err error) {
	return c.r.Read(b)
}

// Write writes data to the underlying io.Writer.
func (c *pipeConn) Write(b []byte) (n int, err error) {
	return c.w.Write(b)
}

// Close calls c if it is not nil.
func (c *pipeConn) Close() error {
	c.mu.Lock()
	closed := c.closed
	c.closed = true
	c.mu.Unlock()

	// Needs to protect from calling Close more than once. For example, grpc-go-1.25.0 calls Close twice.
	if closed {
		return errors.New("pipeConn: Close was already called")
	}

	if c.c == nil {
		return nil
	}
	return c.c()
}

// LocalAddr returns a fake IPv4 address.
func (c *pipeConn) LocalAddr() net.Addr {
	return fakeAddr
}

// RemoteAddr returns a fake IPv4 address.
func (c *pipeConn) RemoteAddr() net.Addr {
	return fakeAddr
}

// SetDeadline always returns not implemented error.
func (c *pipeConn) SetDeadline(t time.Time) error {
	return errNotImpl
}

// SetReadDeadline always returns not implemented error.
func (c *pipeConn) SetReadDeadline(t time.Time) error {
	return errNotImpl
}

// SetWriteDeadline always returns not implemented error.
func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	return errNotImpl
}

var _ net.Conn = (*pipeConn)(nil)

// pipeListener is a pseudo net.Listener implementation based on io.Reader and
// io.Writer. pipeListener's Accept returns exactly one net.Conn that is based
// on the given io.Reader and io.Writer. When the connection is closed, Accept
// returns io.EOF.
//
// pipeListener is suitable for running a gRPC server over a bidirectional pipe.
type pipeListener struct {
	ch chan *pipeConn
}

// newPipeListener constructs a new pipeListener based on r and w.
func newPipeListener(r io.Reader, w io.Writer) *pipeListener {
	connCh := make(chan *pipeConn, 1)
	lis := &pipeListener{ch: connCh}
	conn := &pipeConn{
		r: r,
		w: w,
		c: func() error {
			close(connCh)
			return nil
		},
	}
	connCh <- conn
	return lis
}

// Accept returns a connection. See the comment of pipeListener for its behavior.
func (l *pipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, io.EOF
	}
	return conn, nil
}

// Close closes the listener.
func (l *pipeListener) Close() error {
	return nil
}

// Addr returns a fake IPv4 address.
func (l *pipeListener) Addr() net.Addr {
	return fakeAddr
}

var _ net.Listener = (*pipeListener)(nil)

// newPipeClientConn constructs ClientConn based on r and w.
//
// The returned ClientConn is suitable for talking with a gRPC server over a
// bidirectional pipe.
func newPipeClientConn(ctx context.Context, r io.Reader, w io.Writer, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := append([]grpc.DialOption{
		grpc.WithInsecure(),
		// TODO(crbug.com/989419): Use grpc.WithContextDialer after updating grpc-go.
		grpc.WithDialer(func(string, time.Duration) (net.Conn, error) {
			return &pipeConn{r: r, w: w}, nil
		}),
	}, extraOpts...)
	return grpc.DialContext(ctx, "", opts...)
}
