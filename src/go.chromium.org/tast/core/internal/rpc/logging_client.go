// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"fmt"
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
)

// remoteLoggingClient is a client of the tast.core.Logging gRPC service that
// calls a logging function for every received ReadLogsResponse.
//
// It is used by gRPC clients to receive logs from gRPC services.
type remoteLoggingClient struct {
	stream protocol.Logging_ReadLogsClient
	doneCh chan struct{} // closed when the streaming RPC call is done

	runErr error // set when doneCh is closed

	mu      sync.Mutex        // protects other fields
	lastSeq uint64            // last observed sequence ID
	waiters []chan<- struct{} // channels waiting for logs
}

// newRemoteLoggingClient constructs remoteLoggingClient using conn. logger is
// called for every received ReadLogsResponse.
func newRemoteLoggingClient(ctx context.Context, conn *grpc.ClientConn) (*remoteLoggingClient, error) {
	cl := protocol.NewLoggingClient(conn)
	stream, err := cl.ReadLogs(ctx)
	if err != nil {
		return nil, err
	}

	// Read the initial response to check success and make sure we have been
	// subscribed to logs.
	if _, err := stream.Recv(); err != nil {
		return nil, err
	}

	l := &remoteLoggingClient{
		stream: stream,
		doneCh: make(chan struct{}),
	}

	// Start a goroutine to call logger for every received ReadLogsResponse.
	go l.runBackground(ctx)

	return l, nil
}

// Wait waits until an entry of the specified sequence ID is received.
func (l *remoteLoggingClient) Wait(ctx context.Context, seq uint64) error {
	for {
		l.mu.Lock()
		if l.lastSeq >= seq {
			l.mu.Unlock()
			return nil
		}
		// Wait for next entry.
		waiter := make(chan struct{})
		l.waiters = append(l.waiters, waiter)
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.doneCh:
			if l.runErr == nil {
				return errors.New("logging stream closed")
			}
			return errors.Wrap(l.runErr, "logging stream closed")
		case <-waiter:
		}
	}
}

// Close finishes the remote logging.
func (l *remoteLoggingClient) Close() error {
	l.stream.CloseSend()
	<-l.doneCh
	if l.runErr != nil {
		return errors.Wrap(l.runErr, "remote logging background routine failed")
	}
	return nil
}

func (l *remoteLoggingClient) runBackground(ctx context.Context) {
	defer close(l.doneCh)

	l.runErr = func() error {
		for {
			res, err := l.stream.Recv()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				grpcStatus, ok := status.FromError(err)
				if ok && grpcStatus.Code() == codes.Unavailable {
					logging.Debugf(ctx, "Remote Logging interrupted with following error: %v", grpcStatus)
					return nil
				}
				return err
			}

			switch res.Entry.GetLevel() {
			case protocol.LogLevel_DEBUG:
				logging.Debug(ctx, res.Entry.GetMsg())
			case protocol.LogLevel_INFO:
				logging.Info(ctx, res.Entry.GetMsg())
			default:
				logging.Info(ctx, fmt.Sprintf("UNKNOWN LEVEL:%s", res.Entry.GetMsg()))
			}

			l.mu.Lock()
			l.lastSeq = res.Entry.GetSeq()
			for _, w := range l.waiters {
				close(w)
			}
			l.waiters = nil
			l.mu.Unlock()
		}
	}()
}
