// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"io"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
)

// runRPCServer runs a runner RPC server.
func runRPCServer(scfg *StaticConfig, r io.Reader, w io.Writer) error {
	return rpc.RunServer(r, w, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		protocol.RegisterTestServiceServer(srv, newTestServer(scfg,
			req.GetRunnerInitParams(), req.GetBundleInitParams()))
		return nil
	})
}
