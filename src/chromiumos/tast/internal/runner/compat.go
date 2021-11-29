// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"io"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
)

type compatServer struct {
	cw   *io.PipeWriter
	cr   *io.PipeReader
	conn *rpc.GenericClient
}

func (s *compatServer) Close() {
	s.conn.Close()
	s.cw.Close()
	s.cr.Close()
}

func (s *compatServer) Client() protocol.TestServiceClient {
	return protocol.NewTestServiceClient(s.conn.Conn())
}

// startCompatServer starts an in-process gRPC server to be used to implement
// JSON-based protocol handlers.
func startCompatServer(ctx context.Context, scfg *StaticConfig, req *protocol.HandshakeRequest) (s *compatServer, retErr error) {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	defer func() {
		if retErr != nil {
			cw.Close()
			cr.Close()
		}
	}()
	go runRPCServer(scfg, sr, sw)

	conn, err := rpc.NewClient(ctx, cr, cw, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to in-process gRPC server")
	}

	return &compatServer{
		cw:   cw,
		cr:   cr,
		conn: conn,
	}, nil
}
