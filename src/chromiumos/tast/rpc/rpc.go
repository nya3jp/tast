package rpc

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
)

var (
	fakeAddr = &net.IPAddr{IP: net.IPv4zero}

	errNotImpl = errors.New("not implemented")
)

type pipeConn struct {
	r io.Reader
	w io.Writer
}

func (c *pipeConn) Read(b []byte) (n int, err error) {
	return c.r.Read(b)
}

func (c *pipeConn) Write(b []byte) (n int, err error) {
	return c.w.Write(b)
}

func (c *pipeConn) Close() error {
	return nil
}

func (c *pipeConn) LocalAddr() net.Addr {
	return fakeAddr
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return fakeAddr
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	return errNotImpl
}

func (c *pipeConn) SetReadDeadline(t time.Time) error {
	return errNotImpl
}

func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	return errNotImpl
}

var _ net.Conn = (*pipeConn)(nil)

type PipeListener struct {
	ch chan *pipeConn
}

func NewPipeListener(r io.Reader, w io.Writer) *PipeListener {
	ch := make(chan *pipeConn, 1)
	ch <- &pipeConn{r, w}
	return &PipeListener{ch}
}

func (l *PipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, io.EOF
	}
	return conn, nil
}

func (l *PipeListener) Close() error {
	close(l.ch)
	return nil
}

func (l *PipeListener) Addr() net.Addr {
	return fakeAddr
}

var _ net.Listener = (*PipeListener)(nil)

func NewClientConn(ctx context.Context, r io.Reader, w io.Writer) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithDialer(func(string, time.Duration) (net.Conn, error) {
			return &pipeConn{r, w}, nil
		}),
	}
	return grpc.DialContext(ctx, "", opts...)
}
