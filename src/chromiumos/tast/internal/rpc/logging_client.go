// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"io"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
)

// remoteLoggingClient is a client of the tast.core.Logging gRPC service that
// calls a logging function for every received ReadLogsResponse.
//
// It is used by gRPC clients to receive logs from gRPC services.
type remoteLoggingClient struct {
	stopCh chan struct{} // closed when Close is called to stop the streaming RPC call
	doneCh chan error    // closed when the streaming RPC call is done
}

// newRemoteLoggingClient constructs remoteLoggingClient using conn. logger is
// called for every received ReadLogsResponse.
func newRemoteLoggingClient(ctx context.Context, conn *grpc.ClientConn) (*remoteLoggingClient, error) {
	cl := protocol.NewLoggingClient(conn)
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
				logging.Info(ctx, res.Entry.Msg)
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
