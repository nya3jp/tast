// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../.. ./logging.proto

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

type loggingServerImpl struct {
	mu   sync.Mutex
	seq  int64
	logs chan<- *LogEntry
}

func newLoggingServerImpl() *loggingServerImpl {
	return &loggingServerImpl{}
}

func (s *loggingServerImpl) ReadLogs(srv Logging_ReadLogsServer) error {
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

var _ LoggingServer = (*loggingServerImpl)(nil)

func (s *loggingServerImpl) Log(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs == nil {
		return
	}
	seq := s.seq
	s.seq++
	s.logs <- &LogEntry{Seq: seq, Msg: msg}
}

func (s *loggingServerImpl) Seq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

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

type relayLogger struct {
	stopCh chan struct{}
	doneCh chan struct{}
}

func newRelayLogger(ctx context.Context, conn *grpc.ClientConn) (*relayLogger, error) {
	cl := NewLoggingClient(conn)
	st, err := cl.ReadLogs(ctx)
	if err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		<-stopCh
		st.CloseSend()
	}()

	go func() {
		defer close(doneCh)
		for {
			res, err := st.Recv()
			if err != nil {
				if err != io.EOF {
					testing.ContextLog(ctx, "relay logger aborted: ", err)
				}
				return
			}
			testing.ContextLog(ctx, res.Entry.Msg)
		}
	}()

	return &relayLogger{
		stopCh: stopCh,
		doneCh: doneCh,
	}, nil
}

func (l *relayLogger) Stop() {
	close(l.stopCh)
	<-l.doneCh
}
