// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// RunRPCServer runs the bundle as an RPC server.
func RunRPCServer(r io.Reader, w io.Writer, reg *testing.Registry) error {
	return rpc.RunServer(r, w, reg.AllServices(), func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		registerFixtureService(srv, reg)
		return nil
	})
}
