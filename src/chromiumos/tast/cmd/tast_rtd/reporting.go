// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"io"

	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
)

type logStream struct {
	io.WriteCloser
	name    string
	request string
	stream  *rtd.ProgressSink_ReportLogClient
	conn    *grpc.ClientConn
}

// Write writes the data to the ReportLog RPC stream.
func (s logStream) Write(p []byte) (n int, err error) {
	req := rtd.ReportLogRequest{
		Name:    s.name,
		Request: s.request,
		Data:    p,
	}
	if err := (*s.stream).Send(&req); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the stream and the connection to the RPC service.
func (s logStream) Close() error {
	if _, err := (*s.stream).CloseAndRecv(); err != nil {
		return err
	}
	return (*s.conn).Close()
}

// newReportLogStream establishes a connection to a TLS server.
func newReportLogStream(server, name, requestName string) (io.WriteCloser, error) {
	var err error
	conn, err := grpc.Dial(server, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to establish connection to server: %s", server)
	}
	client := rtd.NewProgressSinkClient(conn)
	stream, err := client.ReportLog(context.Background())
	if err != nil {
		return nil, err
	}
	result := logStream{
		name:    name,
		request: requestName,
		stream:  &stream,
		conn:    conn,
	}
	return result, nil
}
