// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"container/list"
	"errors"
	"sync"
)

// remoteLoggingServer implements the tast.core.Logging gRPC service.
//
// It is provided by gRPC servers to let clients receive logs from gRPC services.
type remoteLoggingServer struct {
	// mu protects fields.
	mu sync.Mutex
	// inbox is a channel to send logs from gRPC servers to. It is nil if there is
	// no client. Sending to this channel will never block.
	inbox chan<- *LogEntry
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

		if s.inbox != nil {
			return errors.New("concurrent ReadLogs calls are disallowed")
		}

		dst, src := make(chan *LogEntry), make(chan *LogEntry)
		logs, s.inbox = dst, src
		go bufferedRelay(dst, src)
		return nil
	}(); err != nil {
		return err
	}

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		close(s.inbox)
		s.inbox = nil
		// Read all remaining logs to stop the bufferedRelay goroutine.
		for range logs {
		}
	}()

	// Send an initial response to notify successful subscription.
	if err := srv.Send(&ReadLogsResponse{}); err != nil {
		return err
	}

	// Stop when the request stream is closed or broken.
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
			if err := srv.Send(&ReadLogsResponse{Entry: e}); err != nil {
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
	if s.inbox == nil {
		return
	}
	s.inbox <- &LogEntry{Msg: msg}
}

// bufferedRelay receives logs from src and sends them to dst keeping the order.
// Even if dst is blocked, it keeps receiving from src by maintaining an internal
// buffer of logs.
// Once src is closed and all buffered logs are sent to dst, it closes dst and
// returns.
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

	close(dst)
}
