// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"

	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
)

// LogStream represents a connection to ReportLog RPC in ProgressSink service.
type LogStream struct {
	name   string
	stream *rtd.ProgressSink_ReportLogClient
	conn   *grpc.ClientConn
}

// Write writes the data to the ReportLog RPC stream.
func (s LogStream) Write(requestName string, p []byte) (err error) {
	req := rtd.ReportLogRequest{
		Name:    s.name,
		Request: requestName,
		Data:    p,
	}
	if err := (*s.stream).Send(&req); err != nil {
		return err
	}
	return nil
}

// Close closes the stream and the connection to the RPC service.
func (s LogStream) Close() error {
	if _, err := (*s.stream).CloseAndRecv(); err != nil {
		return err
	}
	return (*s.conn).Close()
}

// newReportLogStream establishes a connection to a TLS server.
func newReportLogStream(server, name, requestName string) (*LogStream, error) {
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
	result := &LogStream{
		name:   name,
		stream: &stream,
		conn:   conn,
	}
	return result, nil
}
