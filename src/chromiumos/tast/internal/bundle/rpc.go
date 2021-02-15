// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"io"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

func runRPCServer(r io.Reader, w io.Writer, svcs []*testing.Service) error {
	return rpc.RunServer(r, w, svcs, func(srv *grpc.Server) {
		registerFixtureService(srv)
	})
}
