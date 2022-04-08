// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	frameworkprotocol "chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/run/resultsjson"
)

// RPCClient implements a client of the reporting gRPC service.
// nil is a valid RPCClient that discards all reports.
type RPCClient struct {
	conn   *grpc.ClientConn
	stream frameworkprotocol.Reports_LogStreamClient
}

// NewRPCClient creates a new RPCClient that reports test results to the server
// at addr.
// If addr is an empty string, this function returns nil, which is a valid
// RPCClient that discards all reports.
func NewRPCClient(ctx context.Context, addr string) (cl *RPCClient, retErr error) {
	if addr == "" {
		return nil, nil
	}

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			conn.Close()
		}
	}()

	stream, err := frameworkprotocol.NewReportsClient(conn).LogStream(ctx)
	if err != nil {
		return nil, err
	}

	return &RPCClient{
		conn:   conn,
		stream: stream,
	}, nil
}

// Close waits until the server acknowledges delivery of all logs messages, and
// closes the underlying connection of the RPCClient.
func (c *RPCClient) Close() error {
	if c == nil {
		return nil
	}

	var firstErr error
	if err := c.stream.CloseSend(); err != nil && firstErr == nil {
		firstErr = err
	}
	if _, err := c.stream.CloseAndRecv(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// RPCTestLogWriter is an io.Writer that reports written data as test logs via
// the reporting gRPC service.
// nil is a valid RPCTestLogWriter that silently discards all written data.
type RPCTestLogWriter struct {
	stream   frameworkprotocol.Reports_LogStreamClient
	testName string
	logPath  string
}

// Write sends given bytes to the reporting gRPC service.
func (w *RPCTestLogWriter) Write(p []byte) (n int, err error) {
	if w == nil {
		return len(p), nil
	}

	req := frameworkprotocol.LogStreamRequest{
		Test:    w.testName,
		LogPath: w.logPath,
		Data:    p,
	}
	if err := w.stream.Send(&req); err != nil {
		return 0, err
	}
	return len(p), nil
}

// NewTestLogWriter returns an RPCTestLogWriter that reports written data as
// test logs via the reporting gRPC service.
func (c *RPCClient) NewTestLogWriter(testName, logPath string) *RPCTestLogWriter {
	if c == nil {
		return nil
	}

	return &RPCTestLogWriter{
		stream:   c.stream,
		testName: testName,
		logPath:  logPath,
	}
}

// ErrTerminate is returned by ReportResult when the reporting gRPC service
// requested us to terminate testing.
var ErrTerminate = errors.New("reporting service requested to terminate")

// ReportResult reports a test result to the reporting gRPC service.
// It may return ErrTerminate if the server responded with a request to
// terminate testing.
func (c *RPCClient) ReportResult(ctx context.Context, r *resultsjson.Result) error {
	if c == nil {
		return nil
	}

	startTime, err := ptypes.TimestampProto(r.Start)
	if err != nil {
		return err
	}
	duration := ptypes.DurationProto(r.End.Sub(r.Start))
	req := &frameworkprotocol.ReportResultRequest{
		Test:       r.Name,
		SkipReason: r.SkipReason,
		StartTime:  startTime,
		Duration:   duration,
	}
	for _, e := range r.Errors {
		ts, err := ptypes.TimestampProto(e.Time)
		if err != nil {
			return err
		}
		req.Errors = append(req.Errors, &frameworkprotocol.ErrorReport{
			Time:   ts,
			Reason: e.Reason,
			File:   e.File,
			Line:   int32(e.Line),
			Stack:  e.Stack,
		})
	}
	res, err := frameworkprotocol.NewReportsClient(c.conn).ReportResult(ctx, req)
	if err != nil {
		return err
	}
	if res.GetTerminate() {
		return ErrTerminate
	}
	return nil
}
