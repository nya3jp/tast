// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"errors"
	"io"
	"sync"

	"google.golang.org/grpc"
)

// remoteLoggingClient is a client of the tast.core.Logging gRPC service that
// calls a logging function for every received ReadLogsResponse.
//
// It is used by gRPC clients to receive logs from gRPC services.
type remoteLoggingClient struct {
	stopCh chan struct{} // closed when Close is called to stop the streaming RPC call
	doneCh chan error    // closed when the streaming RPC call is done

	mu      sync.Mutex // protects lastSeq and seqCh
	lastSeq int64
	seqCh   chan struct{} // if not nil, when lastSeq is updated, it is closed and set to nil
}

// newRemoteLoggingClient constructs remoteLoggingClient using conn. logger is
// called for every received ReadLogsResponse.
func newRemoteLoggingClient(ctx context.Context, conn *grpc.ClientConn, logger func(msg string)) (*remoteLoggingClient, error) {
	cl := NewLoggingClient(conn)
	st, err := cl.ReadLogs(ctx)
	if err != nil {
		return nil, err
	}

	// Read the initial response to check success and make sure we have been
	// subscribed to logs.
	if _, err := st.Recv(); err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})
	doneCh := make(chan error)

	// Call CloseSend when Close is called.
	go func() {
		<-stopCh
		st.CloseSend()
	}()

	l := &remoteLoggingClient{
		stopCh: stopCh,
		doneCh: doneCh,
	}

	// Start a goroutine to call logger for every received ReadLogsResponse.
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
				if res.Entry == nil {
					return errors.New("ReadLogs returned a ReadLogsResponse with empty entry")
				}
				logger(res.Entry.Msg)
				func() {
					l.mu.Lock()
					defer l.mu.Unlock()
					l.lastSeq = res.Entry.Seq
					if l.seqCh != nil {
						close(l.seqCh)
						l.seqCh = nil
					}
				}()
			}
		}()
	}()

	return l, nil
}

// WaitSeq waits until a log message with a sequence number no less than seq is
// received or context deadline is reached. It returns success if the condition
// is already met even if ctx is already canceled.
func (l *remoteLoggingClient) WaitSeq(ctx context.Context, seq int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for l.lastSeq < seq {
		if l.seqCh == nil {
			l.seqCh = make(chan struct{})
		}
		seqCh := l.seqCh
		if err := func() error {
			l.mu.Unlock()
			defer l.mu.Lock()
			select {
			case <-seqCh:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}(); err != nil {
			return err
		}
	}
	return nil
}

// Close finishes the remote logging.
func (l *remoteLoggingClient) Close() error {
	close(l.stopCh)
	return <-l.doneCh
}
