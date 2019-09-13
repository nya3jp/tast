// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"container/list"
	"context"
	"errors"
	"io"
	"sync"

	"google.golang.org/grpc"

	"chromiumos/tast/testing"
)

// remoteLoggingServer implements the tast.core.Logging gRPC service.
//
// It is provided by gRPC servers to let clients receive logs from gRPC services.
type remoteLoggingServer struct {
	// mu protects fields.
	mu sync.Mutex
	// logs is a channel to send logs. It is nil if there is no client.
	// Sending to this channel will not block.
	logs chan<- *LogEntry
}

func newRemoteLoggingServer() *remoteLoggingServer {
	return &remoteLoggingServer{}
}

func (s *remoteLoggingServer) ReadLogs(srv Logging_ReadLogsServer) error {
	ctx := srv.Context()

	var logs <-chan *LogEntry
	if err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.logs != nil {
			return errors.New("concurrent ReadLogs calls are disallowed")
		}

		dst, src := make(chan *LogEntry), make(chan *LogEntry)
		logs, s.logs = dst, src
		go bufferedRelay(dst, src)
		return nil
	}(); err != nil {
		return err
	}

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		close(s.logs)
		s.logs = nil
		// Read all remaining logs to stop the bufferedRelay goroutine.
		for range logs {
		}
	}()

	// Stop when the client-to-server channel is closed or broken.
	finCh := make(chan struct{})
	go func() {
		defer close(finCh)
		for {
			if _, err := srv.Recv(); err != nil {
				return
			}
			// Discard valid ReadLogsRequest.
		}
	}()

	for {
		select {
		case e := <-logs:
			if err := srv.Send(&ReadLogsReply{Entry: e}); err != nil {
				return err
			}
		case <-finCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

var _ LoggingServer = (*remoteLoggingServer)(nil)

// Log sends msg to connected clients if any.
// This method can be called on any goroutine.
func (s *remoteLoggingServer) Log(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs == nil {
		return
	}
	s.logs <- &LogEntry{Msg: msg}
}

// bufferedRelay receives logs from src and sends them to dst keeping the order.
// Even if dst is blocked, it keeps receiving from src by maintaining an internal
// buffer of logs.
// It returns when src is closed and all buffered logs are sent to dst.
// It panics if dst is closed before all logs are processed.
func bufferedRelay(dst chan<- *LogEntry, src <-chan *LogEntry) {
	var buf list.List

loop:
	for {
		if buf.Len() == 0 {
			msg, ok := <-src
			if !ok {
				break loop
			}
			buf.PushBack(msg)
		}

		select {
		case msg, ok := <-src:
			if !ok {
				break loop
			}
			buf.PushBack(msg)
		case dst <- buf.Front().Value.(*LogEntry):
			buf.Remove(buf.Front())
		}
	}

	for buf.Len() > 0 {
		e := buf.Front()
		dst <- e.Value.(*LogEntry)
		buf.Remove(e)
	}
}

// remoteLoggingServer is a client of the tast.core.Logging gRPC service that
// calls testing.ContextLog for every received ReadLogsReply.
//
// It is used by gRPC clients to receive logs from gRPC services.
type remoteLoggingClient struct {
	stopCh chan struct{} // closed when Close is called to stop the streaming RPC call
	doneCh chan error    // closed when the streaming RPC call is done
}

// newRemoteLoggingClient constructs remoteLoggingClient using conn.
func newRemoteLoggingClient(ctx context.Context, conn *grpc.ClientConn) (*remoteLoggingClient, error) {
	cl := NewLoggingClient(conn)
	st, err := cl.ReadLogs(ctx)
	if err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})
	doneCh := make(chan error)

	// Call CloseSend when Close is called.
	go func() {
		<-stopCh
		st.CloseSend()
	}()

	// Start a goroutine to call testing.ContextLog for every received ReadLogsReply.
	go func() {
		doneCh <- func() error {
			for {
				res, err := st.Recv()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				testing.ContextLog(ctx, res.Entry.Msg)
			}
		}()
	}()

	return &remoteLoggingClient{
		stopCh: stopCh,
		doneCh: doneCh,
	}, nil
}

// Close finishes the remote logging.
func (l *remoteLoggingClient) Close() error {
	close(l.stopCh)
	return <-l.doneCh
}
